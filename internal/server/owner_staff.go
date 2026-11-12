package server

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"bosho/internal/database"
)

func (a *App) ownerStaff(w http.ResponseWriter, r *http.Request, s *Staff) {
	people := database.List(a.db, `SELECT id,name,phone,role FROM staff_users WHERE restaurant_id=? ORDER BY id`,
		func(r *sql.Rows, p *Staff) error { return r.Scan(&p.ID, &p.Name, &p.Phone, &p.Role) }, s.R.ID)
	a.render(w, "owner_staff", M{"Staff": s, "Page": "staff", "People": people, "Err": r.URL.Query().Get("err")})
}

func (a *App) staffCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	name, phone, pass, role := clean(r.FormValue("name"), 40), clean(r.FormValue("phone"), 20), r.FormValue("password"), r.FormValue("role")
	if !strings.Contains(" kitchen waiter cashier owner ", " "+role+" ") {
		role = "waiter"
	}
	if name == "" || len(phone) < 6 || len(pass) < 8 {
		back(w, r, "/owner/staff?err=Fill+in+every+field.+Password+needs+8+characters.")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if _, err := a.db.Exec(`INSERT INTO staff_users(restaurant_id,name,phone,password_hash,role) VALUES(?,?,?,?,?)`, s.R.ID, name, phone, string(hash), role); err != nil {
		back(w, r, "/owner/staff?err=That+phone+number+already+has+an+account.")
		return
	}
	back(w, r, "/owner/staff")
}

func (a *App) staffDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	if id := pathID(r, "id"); id != s.ID {
		a.db.Exec(`DELETE FROM staff_users WHERE id=? AND restaurant_id=?`, id, s.R.ID)
	}
	back(w, r, "/owner/staff")
}

func (a *App) settings(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "owner_settings", M{"Staff": s, "Page": "settings", "Saved": r.URL.Query().Get("saved") != ""})
}

func (a *App) settingsSave(w http.ResponseWriter, r *http.Request, s *Staff) {
	pctOf := func(k string) float64 {
		f, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue(k)), 64)
		if err != nil || f < 0 || f > 100 {
			return 0
		}
		return f
	}
	name := clean(r.FormValue("name"), 60)
	if name == "" {
		name = s.R.Name
	}
	a.db.Exec(`UPDATE restaurants SET name=?,vat_percent=?,service_percent=?,is_open=?,require_accept=? WHERE id=?`,
		name, pctOf("vat"), pctOf("service"), r.FormValue("is_open") == "1", r.FormValue("require_accept") == "1", s.R.ID)
	back(w, r, "/owner/settings?saved=1")
}
