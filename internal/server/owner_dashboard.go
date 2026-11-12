package server

import (
	"database/sql"
	"net/http"
	"time"

	"bosho/internal/database"
)

func (a *App) ownerRoutes(mux *http.ServeMux) {
	o := func(h func(http.ResponseWriter, *http.Request, *Staff)) http.HandlerFunc { return a.auth(true, h) }
	mux.HandleFunc("GET /owner", o(a.dashboard))
	mux.HandleFunc("GET /owner/menu", o(a.ownerMenu))
	mux.HandleFunc("POST /owner/categories", o(a.catCreate))
	mux.HandleFunc("POST /owner/categories/{id}/delete", o(a.catDelete))
	mux.HandleFunc("POST /owner/items", o(a.itemCreate))
	mux.HandleFunc("GET /owner/items/{id}", o(a.itemEdit))
	mux.HandleFunc("POST /owner/items/{id}", o(a.itemUpdate))
	mux.HandleFunc("POST /owner/items/{id}/delete", o(a.itemDelete))
	mux.HandleFunc("POST /owner/items/{id}/options", o(a.optCreate))
	mux.HandleFunc("POST /owner/options/{id}/delete", o(a.optDelete))
	mux.HandleFunc("GET /owner/tables", o(a.ownerTables))
	mux.HandleFunc("POST /owner/tables", o(a.tableCreate))
	mux.HandleFunc("POST /owner/tables/{id}/toggle", o(a.tableToggle))
	mux.HandleFunc("GET /owner/tables/{id}/qr.png", o(a.tableQR))
	mux.HandleFunc("GET /owner/tables/print", o(a.tablePrint))
	mux.HandleFunc("GET /owner/staff", o(a.ownerStaff))
	mux.HandleFunc("POST /owner/staff", o(a.staffCreate))
	mux.HandleFunc("POST /owner/staff/{id}/delete", o(a.staffDelete))
	mux.HandleFunc("GET /owner/settings", o(a.settings))
	mux.HandleFunc("POST /owner/settings", o(a.settingsSave))
}

type methodRow struct {
	Method string
	Count  int
	Amount int64
}

type topRow struct {
	Name string
	Qty  int
}

type reviewRow struct {
	Rating  int
	Comment string
	Table   string
	At      int64
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request, s *Staff) {
	t := time.Now().In(a.loc)
	dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, a.loc).Unix()
	rid := s.R.ID
	methods := database.List(a.db, `SELECT method,COUNT(*),SUM(amount)::BIGINT FROM payments WHERE restaurant_id=? AND created_at>=? GROUP BY method ORDER BY 3 DESC`,
		func(r *sql.Rows, m *methodRow) error { return r.Scan(&m.Method, &m.Count, &m.Amount) }, rid, dayStart)
	var sales int64
	var bills int
	for _, m := range methods {
		sales += m.Amount
		bills += m.Count
	}
	var openBills, tables, items int
	var avg sql.NullFloat64
	a.db.QueryRow(`SELECT COUNT(*) FROM table_sessions WHERE restaurant_id=? AND status IN ('open','bill_requested')`, rid).Scan(&openBills)
	a.db.QueryRow(`SELECT COUNT(*) FROM tables WHERE restaurant_id=?`, rid).Scan(&tables)
	a.db.QueryRow(`SELECT COUNT(*) FROM menu_items WHERE restaurant_id=? AND NOT deleted`, rid).Scan(&items)
	a.db.QueryRow(`SELECT AVG(rating)::FLOAT8 FROM reviews WHERE restaurant_id=? AND menu_item_id IS NULL`, rid).Scan(&avg)
	top := database.List(a.db, `SELECT CASE WHEN i.name_en!='' THEN i.name_en ELSE i.name_bn END,SUM(i.quantity) FROM order_items i JOIN orders o ON o.id=i.order_id
		WHERE o.restaurant_id=? AND o.placed_at>=? AND o.status NOT IN ('rejected','cancelled') AND i.status!='cancelled'
		GROUP BY i.menu_item_id,i.name_en,i.name_bn ORDER BY 2 DESC LIMIT 5`,
		func(r *sql.Rows, t *topRow) error { return r.Scan(&t.Name, &t.Qty) }, rid, now()-30*86400)
	reviews := database.List(a.db, `SELECT v.rating,v.comment,t.label,v.created_at FROM reviews v JOIN table_sessions s ON s.id=v.session_id JOIN tables t ON t.id=s.table_id
		WHERE v.restaurant_id=? AND v.menu_item_id IS NULL ORDER BY v.id DESC LIMIT 10`,
		func(r *sql.Rows, v *reviewRow) error { return r.Scan(&v.Rating, &v.Comment, &v.Table, &v.At) }, rid)
	a.render(w, "dashboard", M{"Staff": s, "Page": "owner", "Sales": sales, "BillsPaid": bills, "Methods": methods,
		"OpenBills": openBills, "Tables": tables, "Items": items, "Avg": avg, "Top": top, "Reviews": reviews})
}
