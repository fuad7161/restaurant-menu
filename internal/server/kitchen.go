package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"

	"bosho/internal/database"
)

type callRow struct {
	ID                   int64
	Table, Group, Reason string
	At                   int64
}

type changeRow struct {
	ID                 int64
	Table, Group, Name string
	Qty, ReqQty        int
}

func (a *App) boardData(s *Staff) M {
	orders := a.loadOrders(`o.restaurant_id=? AND o.status IN ('placed','cooking','ready')`, s.R.ID)
	var fresh, cooking, ready []OView
	for _, o := range orders {
		switch o.Status {
		case "placed":
			o.Minutes = estimateMinutes(a.db, s.R.ID, o.ID)
			fresh = append(fresh, o)
		case "cooking":
			if rem := (o.Est - now()) / 60; rem >= 0 {
				o.ETA = fmt.Sprintf("due in %d min", rem)
			} else {
				o.ETA = fmt.Sprintf("late by %d min", -rem)
			}
			cooking = append(cooking, o)
		case "ready":
			ready = append(ready, o)
		}
	}
	calls := database.List(a.db, `SELECT c.id,t.label,s.group_label,c.reason,c.created_at FROM waiter_calls c
		JOIN table_sessions s ON s.id=c.session_id JOIN tables t ON t.id=s.table_id
		WHERE c.restaurant_id=? AND c.status='pending' ORDER BY c.id`,
		func(r *sql.Rows, c *callRow) error { return r.Scan(&c.ID, &c.Table, &c.Group, &c.Reason, &c.At) }, s.R.ID)
	changes := database.List(a.db, `SELECT i.id,t.label,s.group_label,CASE WHEN i.name_en!='' THEN i.name_en ELSE i.name_bn END,i.quantity,i.req_qty
		FROM order_items i JOIN orders o ON o.id=i.order_id JOIN table_sessions s ON s.id=o.session_id JOIN tables t ON t.id=s.table_id
		WHERE o.restaurant_id=? AND i.status='cancel_requested' ORDER BY i.id`,
		func(r *sql.Rows, c *changeRow) error {
			return r.Scan(&c.ID, &c.Table, &c.Group, &c.Name, &c.Qty, &c.ReqQty)
		}, s.R.ID)
	return M{"Staff": s, "New": fresh, "Cooking": cooking, "Ready": ready, "Calls": calls, "Changes": changes,
		"Alert": len(fresh) + len(calls) + len(changes), "Page": "kitchen"}
}

func (a *App) kitchen(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "kitchen", a.boardData(s))
}

func (a *App) kitchenBoard(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "k_board", a.boardData(s))
}

func (a *App) orderAct(w http.ResponseWriter, r *http.Request, s *Staff) {
	id, rid := pathID(r, "id"), s.R.ID
	switch r.PathValue("act") {
	case "accept":
		mins, _ := strconv.Atoi(r.FormValue("minutes"))
		if mins < 1 || mins > 240 {
			mins = estimateMinutes(a.db, rid, id)
		}
		a.db.Exec(`UPDATE orders SET status='cooking', estimated_ready_at=? WHERE id=? AND restaurant_id=? AND status='placed'`, now()+int64(mins)*60, id, rid)
	case "reject":
		reason := clean(r.Header.Get("HX-Prompt"), 120)
		if reason == "" {
			reason = "The kitchen could not take this order."
		}
		a.db.Exec(`UPDATE orders SET status='rejected', reject_reason=? WHERE id=? AND restaurant_id=? AND status IN ('placed','cooking')`, reason, id, rid)
	case "ready":
		a.db.Exec(`UPDATE orders SET status='ready', ready_at=? WHERE id=? AND restaurant_id=? AND status='cooking'`, now(), id, rid)
	case "served":
		a.db.Exec(`UPDATE orders SET status='served' WHERE id=? AND restaurant_id=? AND status='ready'`, id, rid)
		// A change request that nobody answered lapses once the food is on the table.
		a.db.Exec(`UPDATE order_items SET status='active', last_change='declined' WHERE order_id=? AND status='cancel_requested'
			AND EXISTS (SELECT 1 FROM orders WHERE id=? AND restaurant_id=? AND status='served')`, id, id, rid)
	}
	a.render(w, "k_board", a.boardData(s))
}

// itemAct handles guest change requests (approve, decline) and staff voids on order lines.
func (a *App) itemAct(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	var oid int64
	var status string
	var req int
	err := a.db.QueryRow(`SELECT o.id,i.status,i.req_qty FROM order_items i JOIN orders o ON o.id=i.order_id WHERE i.id=? AND o.restaurant_id=?`, id, s.R.ID).
		Scan(&oid, &status, &req)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	act := r.PathValue("act")
	switch {
	case act == "approve" && status == "cancel_requested" && req > 0:
		a.db.Exec(`UPDATE order_items SET quantity=?, status='active', last_change='approved' WHERE id=?`, req, id)
	case act == "approve" && status == "cancel_requested":
		a.db.Exec(`UPDATE order_items SET status='cancelled', last_change='approved', cancelled_by=?, cancel_reason='guest request' WHERE id=?`, s.Name, id)
		closeEmptyOrder(a.db, oid)
	case act == "decline" && status == "cancel_requested":
		a.db.Exec(`UPDATE order_items SET status='active', last_change='declined' WHERE id=?`, id)
	case act == "void" && status != "cancelled":
		reason := clean(r.Header.Get("HX-Prompt"), 120)
		if reason == "" {
			reason = "no reason given"
		}
		// Every removal from a bill records who did it and why.
		a.db.Exec(`UPDATE order_items SET status='cancelled', cancelled_by=?, cancel_reason=? WHERE id=?`, s.Name, reason, id)
		closeEmptyOrder(a.db, oid)
		w.Header().Set("HX-Refresh", "true")
		return
	}
	a.render(w, "k_board", a.boardData(s))
}

func (a *App) callDone(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.db.Exec(`UPDATE waiter_calls SET status='done' WHERE id=? AND restaurant_id=?`, pathID(r, "id"), s.R.ID)
	a.render(w, "k_board", a.boardData(s))
}

func (a *App) stock(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "stock", M{"Staff": s, "Cats": a.menu(s.R.ID), "Page": "stock"})
}

func (a *App) stockToggle(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	a.db.Exec(`UPDATE menu_items SET is_available=NOT is_available WHERE id=? AND restaurant_id=?`, id, s.R.ID)
	if it, ok := a.item(s.R.ID, id); ok {
		a.render(w, "stock_row", it)
	}
}
