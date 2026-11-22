package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"bosho/internal/database"
	"bosho/internal/money"
)

type billRow struct {
	ID                           int64
	Table, Group, Status, Guests string
	OpenedAt, Total              int64
}

func (a *App) counterData(s *Staff) M {
	rows := database.List(a.db, `SELECT s.id,t.label,s.group_label,s.status,s.opened_at,
		COALESCE((SELECT STRING_AGG(display_name, ', ' ORDER BY g.id) FROM session_guests g WHERE g.session_id=s.id AND g.status='joined'),'')
		FROM table_sessions s JOIN tables t ON t.id=s.table_id
		WHERE s.restaurant_id=? AND s.status IN ('open','bill_requested')
		ORDER BY (s.status='bill_requested') DESC, t.label, s.group_label`,
		func(r *sql.Rows, b *billRow) error {
			return r.Scan(&b.ID, &b.Table, &b.Group, &b.Status, &b.OpenedAt, &b.Guests)
		}, s.R.ID)
	// Every open bill's subtotal in one query, worked out exactly as bill() does.
	type sub struct {
		Session int64
		Amount  int64
	}
	subs := map[int64]int64{}
	for _, x := range database.List(a.db, `SELECT o.session_id,SUM(i.quantity*i.unit_price)::BIGINT FROM order_items i JOIN orders o ON o.id=i.order_id
		JOIN table_sessions s ON s.id=o.session_id
		WHERE s.restaurant_id=? AND s.status IN ('open','bill_requested') AND o.status NOT IN ('rejected','cancelled') AND i.status!='cancelled'
		GROUP BY o.session_id`,
		func(r *sql.Rows, x *sub) error { return r.Scan(&x.Session, &x.Amount) }, s.R.ID) {
		subs[x.Session] = x.Amount
	}
	asking := 0
	for i := range rows {
		st := subs[rows[i].ID]
		rows[i].Total = st + money.Percent(st, s.R.Service) + money.Percent(st, s.R.VAT)
		if rows[i].Status == "bill_requested" {
			asking++
		}
	}
	return M{"Staff": s, "Bills": rows, "Alert": asking, "Page": "counter"}
}

func (a *App) counter(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "counter", a.counterData(s))
}

func (a *App) counterList(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.renderPoll(w, r, "c_list", a.counterData(s))
}

func (a *App) sessionRow(s *Staff, id int64) (billRow, bool) {
	var b billRow
	err := a.db.QueryRow(`SELECT s.id,t.label,s.group_label,s.status,s.opened_at FROM table_sessions s JOIN tables t ON t.id=s.table_id
		WHERE s.id=? AND s.restaurant_id=?`, id, s.R.ID).Scan(&b.ID, &b.Table, &b.Group, &b.Status, &b.OpenedAt)
	return b, err == nil
}

func (a *App) sessionDetail(w http.ResponseWriter, r *http.Request, s *Staff) {
	b, ok := a.sessionRow(s, pathID(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	a.render(w, "session", M{"Staff": s, "B": b, "Orders": a.loadOrders(`o.session_id=?`, b.ID), "Bill": a.bill(b.ID, s.R), "Page": "counter"})
}

func (a *App) sessionPaid(w http.ResponseWriter, r *http.Request, s *Staff) {
	b, ok := a.sessionRow(s, pathID(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	method := r.FormValue("method")
	if !strings.Contains(" cash bkash nagad card ", " "+method+" ") {
		method = "cash"
	}
	if b.Status == "open" || b.Status == "bill_requested" {
		total := a.bill(b.ID, s.R).Total
		paid := false
		// One transaction: the group is paid and the payment recorded together, or neither.
		// The status check in the UPDATE makes a second tap (or a second cashier) do nothing.
		err := a.inTx(func(q database.Querier) error {
			res, err := q.Exec(`UPDATE table_sessions SET status='paid', closed_at=? WHERE id=? AND status IN ('open','bill_requested')`, now(), b.ID)
			if rows(res, err) != 1 {
				return err
			}
			paid = true
			if err := exec(q, `INSERT INTO payments(restaurant_id,session_id,amount,method,marked_by,created_at) VALUES(?,?,?,?,?,?)`,
				s.R.ID, b.ID, total, method, s.Name, now()); err != nil {
				return err
			}
			if err := exec(q, `DELETE FROM cart_items WHERE session_id=?`, b.ID); err != nil {
				return err
			}
			return exec(q, `UPDATE waiter_calls SET status='done', done_at=? WHERE session_id=? AND status='pending'`, now(), b.ID)
		})
		if err != nil {
			serverError(w, err)
			return
		}
		if paid {
			a.audit(s.R.ID, s.Name, "bill paid", fmt.Sprintf("table %s group %s: %s by %s", b.Table, b.Group, money.Format(total), method))
		}
	}
	http.Redirect(w, r, "/staff/counter", http.StatusSeeOther)
}

// sessionClose closes a group without payment, for example guests who scanned and left.
func (a *App) sessionClose(w http.ResponseWriter, r *http.Request, s *Staff) {
	if b, ok := a.sessionRow(s, pathID(r, "id")); ok && (b.Status == "open" || b.Status == "bill_requested") {
		total := a.bill(b.ID, s.R).Total
		err := a.inTx(func(q database.Querier) error {
			res, err := q.Exec(`UPDATE table_sessions SET status='closed', closed_at=? WHERE id=? AND status IN ('open','bill_requested')`, now(), b.ID)
			if rows(res, err) != 1 {
				return err
			}
			if err := exec(q, `UPDATE orders SET status='cancelled', reject_reason=? WHERE session_id=? AND status IN ('placed','cooking','ready')`, "closed by "+s.Name, b.ID); err != nil {
				return err
			}
			if err := exec(q, `DELETE FROM cart_items WHERE session_id=?`, b.ID); err != nil {
				return err
			}
			return exec(q, `UPDATE waiter_calls SET status='done', done_at=? WHERE session_id=? AND status='pending'`, now(), b.ID)
		})
		if err != nil {
			serverError(w, err)
			return
		}
		a.audit(s.R.ID, s.Name, "closed without payment", fmt.Sprintf("table %s group %s, bill was %s", b.Table, b.Group, money.Format(total)))
	}
	http.Redirect(w, r, "/staff/counter", http.StatusSeeOther)
}
