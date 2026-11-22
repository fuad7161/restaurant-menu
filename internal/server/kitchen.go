package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"bosho/internal/database"
	"bosho/internal/i18n"
	"bosho/internal/money"
)

type callRow struct {
	ID                   int64
	Table, Group, Reason string
	At                   int64
}

type changeRow struct {
	ID                   int64
	Table, Group, NameBn string
	NameEn               string
	Qty, ReqQty          int
}

// ageClass colours a kitchen ticket by how long it has waited: amber after the restaurant's warn
// limit, red after its late limit (Settings).
func ageClass(r Restaurant, since int64) string {
	mins := int((now() - since) / 60)
	switch {
	case r.LateMinutes > 0 && mins >= r.LateMinutes:
		return "late"
	case r.WarnMinutes > 0 && mins >= r.WarnMinutes:
		return "warn"
	}
	return "ok"
}

func (a *App) boardData(s *Staff) M {
	orders := a.loadOrders(`o.restaurant_id=? AND o.status IN ('placed','cooking','ready')`, s.R.ID)
	var waiting []int64
	for _, o := range orders {
		if o.Status == "placed" {
			waiting = append(waiting, o.ID)
		}
	}
	est := a.estimateAll(s.R.ID, waiting)
	var fresh, cooking, ready []OView
	for _, o := range orders {
		switch o.Status {
		case "placed":
			if o.Minutes = est[o.ID]; o.Minutes == 0 {
				o.Minutes = 15
			}
			o.Age = ageClass(s.R, o.PlacedAt)
			o.ETA = fmt.Sprintf(i18n.T(s.Lang, "s_waiting"), (now()-o.PlacedAt)/60)
			fresh = append(fresh, o)
		case "cooking":
			if rem := (o.Est - now()) / 60; rem >= 0 {
				o.ETA = fmt.Sprintf(i18n.T(s.Lang, "s_due_in"), rem)
			} else {
				o.ETA = fmt.Sprintf(i18n.T(s.Lang, "s_late_by"), -rem)
			}
			o.Age = ageClass(s.R, o.PlacedAt)
			cooking = append(cooking, o)
		case "ready": // waiting to be carried to the table
			o.Age = ageClass(s.R, o.ReadyAt)
			o.ETA = fmt.Sprintf(i18n.T(s.Lang, "s_waiting"), (now()-o.ReadyAt)/60)
			ready = append(ready, o)
		}
	}
	calls := database.List(a.db, `SELECT c.id,t.label,s.group_label,c.reason,c.created_at FROM waiter_calls c
		JOIN table_sessions s ON s.id=c.session_id JOIN tables t ON t.id=s.table_id
		WHERE c.restaurant_id=? AND c.status='pending' ORDER BY c.id`,
		func(r *sql.Rows, c *callRow) error { return r.Scan(&c.ID, &c.Table, &c.Group, &c.Reason, &c.At) }, s.R.ID)
	changes := database.List(a.db, `SELECT i.id,t.label,s.group_label,i.name_bn,i.name_en,i.quantity,i.req_qty
		FROM order_items i JOIN orders o ON o.id=i.order_id JOIN table_sessions s ON s.id=o.session_id JOIN tables t ON t.id=s.table_id
		WHERE o.restaurant_id=? AND i.status='cancel_requested' ORDER BY i.id`,
		func(r *sql.Rows, c *changeRow) error {
			return r.Scan(&c.ID, &c.Table, &c.Group, &c.NameBn, &c.NameEn, &c.Qty, &c.ReqQty)
		}, s.R.ID)
	return M{"Staff": s, "New": fresh, "Cooking": cooking, "Ready": ready, "Calls": calls, "Changes": changes,
		"Alert": len(fresh) + len(calls) + len(changes), "Page": "kitchen"}
}

func (a *App) kitchen(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "kitchen", a.boardData(s))
}

func (a *App) kitchenBoard(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.renderPoll(w, r, "k_board", a.boardData(s))
}

// undoWindow is how many seconds staff have to undo "Mark ready" or "Mark served".
const undoWindow = 60

// staffReason is the text typed into a reason dialog: the form field "reason", or the HX-Prompt
// header that htmx's own prompt sends.
func staffReason(r *http.Request) string {
	if v := clean(r.FormValue("reason"), 120); v != "" {
		return v
	}
	return clean(r.Header.Get("HX-Prompt"), 120)
}

func (a *App) orderAct(w http.ResponseWriter, r *http.Request, s *Staff) {
	id, rid := pathID(r, "id"), s.R.ID
	toast, undo := "", false
	var err error
	switch act := r.PathValue("act"); act {
	case "accept":
		mins, _ := strconv.Atoi(r.FormValue("minutes"))
		if mins < 1 || mins > 240 {
			mins = estimateMinutes(a.db, rid, id)
		}
		err = exec(a.db, `UPDATE orders SET status='cooking', estimated_ready_at=? WHERE id=? AND restaurant_id=? AND status='placed'`, now()+int64(mins)*60, id, rid)
	case "reject":
		reason := staffReason(r)
		if reason == "" {
			reason = "The kitchen could not take this order."
		}
		var res sql.Result
		res, err = a.db.Exec(`UPDATE orders SET status='rejected', reject_reason=? WHERE id=? AND restaurant_id=? AND status IN ('placed','cooking')`, reason, id, rid)
		if rows(res, err) == 1 {
			a.audit(rid, s.Name, "order rejected", fmt.Sprintf("table %s, order #%d: %s", a.orderTable(id), id, reason))
		}
	case "ready":
		var res sql.Result
		res, err = a.db.Exec(`UPDATE orders SET status='ready', ready_at=? WHERE id=? AND restaurant_id=? AND status='cooking'`, now(), id, rid)
		if rows(res, err) == 1 {
			toast, undo = fmt.Sprintf(i18n.T(s.Lang, "s_marked_ready"), a.orderTable(id)), true
		}
	case "served":
		var n int64
		err = a.inTx(func(q database.Querier) error {
			res, err := q.Exec(`UPDATE orders SET status='served', served_at=? WHERE id=? AND restaurant_id=? AND status='ready'`, now(), id, rid)
			if n = rows(res, err); err != nil || n == 0 {
				return err
			}
			// A change request that nobody answered lapses once the food is on the table.
			return exec(q, `UPDATE order_items SET status='active', last_change='declined' WHERE order_id=? AND status='cancel_requested'`, id)
		})
		if err == nil && n == 1 {
			toast, undo = fmt.Sprintf(i18n.T(s.Lang, "s_marked_served"), a.orderTable(id)), true
		}
	case "undo":
		// Step back one status, only shortly after the tap: served -> ready, ready -> cooking.
		toast = i18n.T(s.Lang, "s_undo_late")
		res, e := a.db.Exec(`UPDATE orders SET status='ready', served_at=0 WHERE id=? AND restaurant_id=? AND status='served' AND served_at>?`, id, rid, now()-undoWindow)
		if rows(res, e) == 0 {
			res, e = a.db.Exec(`UPDATE orders SET status='cooking', ready_at=0 WHERE id=? AND restaurant_id=? AND status='ready' AND ready_at>?`, id, rid, now()-undoWindow)
		}
		if rows(res, e) == 1 {
			toast = i18n.T(s.Lang, "s_undone")
		}
		err = e
	}
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, "k_board", a.boardData(s))
	if toast != "" {
		undoURL := ""
		if undo {
			undoURL = fmt.Sprintf("/staff/orders/%d/undo", id)
		}
		a.renderToast(w, s.Lang, toast, undoURL, "#board")
	}
}

// rows is the number of rows an Exec changed, 0 on error.
func rows(res sql.Result, err error) int64 {
	if err != nil {
		return 0
	}
	n, _ := res.RowsAffected()
	return n
}

// orderTable is the table label of an order, for staff messages.
func (a *App) orderTable(orderID int64) string {
	var label string
	a.db.QueryRow(`SELECT t.label FROM orders o JOIN table_sessions s ON s.id=o.session_id JOIN tables t ON t.id=s.table_id WHERE o.id=?`, orderID).Scan(&label)
	return label
}

// itemAct handles guest change requests (approve, decline) and staff voids on order lines.
// itemAct handles guest change requests (approve, decline) and staff voids on order lines.
func (a *App) itemAct(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	var oid int64
	var status, name string
	var req, qty int
	var unit int64
	err := a.db.QueryRow(`SELECT o.id,i.status,i.req_qty,i.quantity,i.unit_price,CASE WHEN i.name_en!='' THEN i.name_en ELSE i.name_bn END
		FROM order_items i JOIN orders o ON o.id=i.order_id WHERE i.id=? AND o.restaurant_id=?`, id, s.R.ID).
		Scan(&oid, &status, &req, &qty, &unit, &name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	act := r.PathValue("act")
	switch {
	case act == "approve" && status == "cancel_requested" && req > 0:
		err = exec(a.db, `UPDATE order_items SET quantity=?, status='active', last_change='approved' WHERE id=? AND status='cancel_requested'`, req, id)
	case act == "approve" && status == "cancel_requested":
		err = a.inTx(func(q database.Querier) error {
			if err := exec(q, `UPDATE order_items SET status='cancelled', last_change='approved', cancelled_by=?, cancel_reason='guest request' WHERE id=? AND status='cancel_requested'`, s.Name, id); err != nil {
				return err
			}
			return closeEmptyOrder(q, oid)
		})
	case act == "decline" && status == "cancel_requested":
		err = exec(a.db, `UPDATE order_items SET status='active', last_change='declined' WHERE id=? AND status='cancel_requested'`, id)
	case act == "void" && status != "cancelled":
		reason := staffReason(r)
		if reason == "" {
			reason = "no reason given"
		}
		// Every removal from a bill records who did it and why.
		err = a.inTx(func(q database.Querier) error {
			if err := exec(q, `UPDATE order_items SET status='cancelled', cancelled_by=?, cancel_reason=? WHERE id=? AND status!='cancelled'`, s.Name, reason, id); err != nil {
				return err
			}
			return closeEmptyOrder(q, oid)
		})
		if err != nil {
			serverError(w, err)
			return
		}
		a.audit(s.R.ID, s.Name, "item removed from bill", fmt.Sprintf("table %s: %d × %s (%s): %s", a.orderTable(oid), qty, name, money.Format(int64(qty)*unit), reason))
		w.Header().Set("HX-Refresh", "true")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, "k_board", a.boardData(s))
}

func (a *App) callDone(w http.ResponseWriter, r *http.Request, s *Staff) {
	if err := exec(a.db, `UPDATE waiter_calls SET status='done', done_at=? WHERE id=? AND restaurant_id=? AND status='pending'`, now(), pathID(r, "id"), s.R.ID); err != nil {
		serverError(w, err)
		return
	}
	a.render(w, "k_board", a.boardData(s))
}

func (a *App) stock(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "stock", M{"Staff": s, "Cats": a.menu(s.R.ID), "Page": "stock"})
}

func (a *App) stockToggle(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	if err := exec(a.db, `UPDATE menu_items SET is_available=NOT is_available WHERE id=? AND restaurant_id=?`, id, s.R.ID); err != nil {
		serverError(w, err)
		return
	}
	if it, ok := a.item(s.R.ID, id); ok {
		a.render(w, "stock_row", M{"It": it, "L": s.Lang})
	}
}
