package server

import (
	"database/sql"
	"net/http"
	"strings"

	"bosho/internal/database"
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
	asking := 0
	for i := range rows {
		rows[i].Total = a.bill(rows[i].ID, s.R).Total
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
	a.render(w, "c_list", a.counterData(s))
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
		tx, err := a.db.Begin()
		if err == nil {
			res, _ := tx.Exec(`UPDATE table_sessions SET status='paid', closed_at=? WHERE id=? AND status IN ('open','bill_requested')`, now(), b.ID)
			if n, _ := res.RowsAffected(); n == 1 {
				tx.Exec(`INSERT INTO payments(restaurant_id,session_id,amount,method,marked_by,created_at) VALUES(?,?,?,?,?,?)`,
					s.R.ID, b.ID, a.bill(b.ID, s.R).Total, method, s.Name, now())
				tx.Exec(`DELETE FROM cart_items WHERE session_id=?`, b.ID)
				tx.Exec(`UPDATE waiter_calls SET status='done' WHERE session_id=?`, b.ID)
			}
			tx.Commit()
		}
	}
	http.Redirect(w, r, "/staff/counter", http.StatusSeeOther)
}

// sessionClose closes a group without payment, for example guests who scanned and left.
func (a *App) sessionClose(w http.ResponseWriter, r *http.Request, s *Staff) {
	if b, ok := a.sessionRow(s, pathID(r, "id")); ok && (b.Status == "open" || b.Status == "bill_requested") {
		a.db.Exec(`UPDATE table_sessions SET status='closed', closed_at=? WHERE id=?`, now(), b.ID)
		a.db.Exec(`UPDATE orders SET status='cancelled', reject_reason=? WHERE session_id=? AND status IN ('placed','cooking','ready')`, "closed by "+s.Name, b.ID)
		a.db.Exec(`DELETE FROM cart_items WHERE session_id=?`, b.ID)
		a.db.Exec(`UPDATE waiter_calls SET status='done' WHERE session_id=?`, b.ID)
	}
	http.Redirect(w, r, "/staff/counter", http.StatusSeeOther)
}
