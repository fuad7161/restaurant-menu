package server

import (
	"database/sql"
	"net/http"
	"time"

	"bosho/internal/database"
)

func (a *App) ownerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /owner", a.need(PermDashboard, a.dashboard))
	mux.HandleFunc("GET /owner/menu", a.need(PermMenuEdit, a.ownerMenu))
	mux.HandleFunc("POST /owner/categories", a.need(PermMenuEdit, a.catCreate))
	mux.HandleFunc("POST /owner/categories/{id}/delete", a.need(PermMenuEdit, a.catDelete))
	mux.HandleFunc("POST /owner/items", a.need(PermMenuEdit, a.itemCreate))
	mux.HandleFunc("GET /owner/items/{id}", a.need(PermMenuEdit, a.itemEdit))
	mux.HandleFunc("POST /owner/items/{id}", a.need(PermMenuEdit, a.itemUpdate))
	mux.HandleFunc("POST /owner/items/{id}/delete", a.need(PermMenuEdit, a.itemDelete))
	mux.HandleFunc("POST /owner/items/{id}/options", a.need(PermMenuEdit, a.optCreate))
	mux.HandleFunc("POST /owner/options/{id}/delete", a.need(PermMenuEdit, a.optDelete))
	mux.HandleFunc("POST /owner/items/{id}/groups", a.need(PermMenuEdit, a.groupCreate))
	mux.HandleFunc("POST /owner/groups/{id}/delete", a.need(PermMenuEdit, a.groupDelete))
	mux.HandleFunc("GET /owner/tables", a.need(PermTablesManage, a.ownerTables))
	mux.HandleFunc("POST /owner/tables", a.need(PermTablesManage, a.tableCreate))
	mux.HandleFunc("POST /owner/tables/{id}/toggle", a.need(PermTablesManage, a.tableToggle))
	mux.HandleFunc("GET /owner/tables/{id}/qr.png", a.need(PermTablesManage, a.tableQR))
	mux.HandleFunc("GET /owner/tables/print", a.need(PermTablesManage, a.tablePrint))
	mux.HandleFunc("POST /owner/tables/{id}/newqr", a.need(PermTablesManage, a.tableNewQR))
	mux.HandleFunc("GET /owner/staff", a.need(PermStaffManage, a.ownerStaff))
	mux.HandleFunc("POST /owner/staff", a.need(PermStaffManage, a.staffCreate))
	mux.HandleFunc("POST /owner/staff/{id}/delete", a.need(PermStaffManage, a.staffDelete))
	mux.HandleFunc("POST /owner/staff/{id}/password", a.need(PermStaffManage, a.staffPassword))
	mux.HandleFunc("GET /owner/activity", a.need(PermActivity, a.activity))
	mux.HandleFunc("GET /owner/reports", a.need(PermReports, a.reports))
	mux.HandleFunc("GET /owner/closing", a.need(PermClosingReport, a.closing))
	mux.HandleFunc("GET /owner/export/{what}", a.need(PermReports, a.export))
	mux.HandleFunc("GET /owner/settings", a.need(PermSettings, a.settings))
	mux.HandleFunc("POST /owner/settings", a.need(PermSettings, a.settingsSave))
	mux.HandleFunc("GET /owner/networks", a.need(PermNetworks, a.networks))
	mux.HandleFunc("POST /owner/networks", a.need(PermNetworks, a.networksSave))
	mux.HandleFunc("GET /owner/profile", a.need(PermProfile, a.profile))
	mux.HandleFunc("POST /owner/profile", a.need(PermProfile, a.profileSave))
	mux.HandleFunc("POST /owner/profile/logo/delete", a.need(PermProfile, a.logoDelete))
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
	a.render(w, "dashboard", M{"Staff": s, "Page": "owner", "AwaitingEmail": a.awaitingEmail(s.R), "Sales": sales, "BillsPaid": bills, "Methods": methods,
		"OpenBills": openBills, "Tables": tables, "Items": items, "Avg": avg, "Top": top, "Reviews": reviews})
}
