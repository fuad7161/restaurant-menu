package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"bosho/internal/database"
)

// staffRow is one person on the Staff page. Password is the staff password the owner set, so the
// owner can look it up; it is empty for owners and for passwords set before this was kept.
type staffRow struct {
	ID                int64
	Name, Phone, Role string
	Password          string
	CanManage         bool // the viewer may remove them and set their password
}

func (a *App) ownerStaff(w http.ResponseWriter, r *http.Request, s *Staff) {
	people := database.List(a.db, `SELECT id,name,phone,role,password_enc FROM staff_users WHERE restaurant_id=? ORDER BY id`,
		func(r *sql.Rows, p *staffRow) error { return r.Scan(&p.ID, &p.Name, &p.Phone, &p.Role, &p.Password) }, s.R.ID)
	for i := range people {
		people[i].CanManage = s.CanManage(people[i].Role)
		if people[i].CanManage { // a manager never sees a manager's or the owner's password
			people[i].Password = a.openPassword(people[i].Password)
		} else {
			people[i].Password = ""
		}
	}
	w.Header().Set("Cache-Control", "no-store") // the page shows passwords
	a.render(w, "owner_staff", M{"Staff": s, "Page": "staff", "People": people, "Err": r.URL.Query().Get("err"), "OK": r.URL.Query().Get("ok") != ""})
}

func (a *App) staffCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	name, phone, pass, role := clean(r.FormValue("name"), 40), clean(r.FormValue("phone"), 20), r.FormValue("password"), r.FormValue("role")
	if !s.CanManage(role) {
		back(w, r, "/owner/staff?err=Choose+one+of+the+roles+in+the+list.")
		return
	}
	if name == "" || len(phone) < 6 || len(pass) < 8 {
		back(w, r, "/owner/staff?err=Fill+in+every+field.+Password+needs+8+characters.")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if _, err := a.db.Exec(`INSERT INTO staff_users(restaurant_id,name,phone,password_hash,password_enc,role) VALUES(?,?,?,?,?,?)`, s.R.ID, name, phone, string(hash), a.sealPassword(pass), role); err != nil {
		back(w, r, "/owner/staff?err=That+phone+number+already+has+an+account.")
		return
	}
	a.audit(s.R.ID, s.Name, "staff added", fmt.Sprintf("%s (%s, %s)", name, phone, role))
	back(w, r, "/owner/staff")
}

func (a *App) staffDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	if id := pathID(r, "id"); id != s.ID {
		var name, role string
		a.db.QueryRow(`SELECT name,role FROM staff_users WHERE id=? AND restaurant_id=?`, id, s.R.ID).Scan(&name, &role)
		// Their logins end with the account (staff_sessions cascade). An owner account can't be
		// removed here: only the platform admin changes who owns a restaurant. A manager removes
		// only kitchen, waiter and cashier.
		if s.CanManage(role) {
			if res, err := a.db.Exec(`DELETE FROM staff_users WHERE id=? AND restaurant_id=? AND role=?`, id, s.R.ID, role); rows(res, err) == 1 {
				a.audit(s.R.ID, s.Name, "staff removed", name)
			}
		}
	}
	back(w, r, "/owner/staff")
}

// staffPassword lets the owner set a new password for a staff member who forgot theirs. It also
// logs that person out everywhere, so an old password left on a shared phone stops working.
// Another owner's password can't be set here (that would let one owner take over the other's
// account); the owner's own password can.
func (a *App) staffPassword(w http.ResponseWriter, r *http.Request, s *Staff) {
	id, pass := pathID(r, "id"), r.FormValue("password")
	if len(pass) < 8 {
		back(w, r, "/owner/staff?err=The+new+password+needs+8+characters.")
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	sealed := a.sealPassword(pass)
	if id == s.ID {
		sealed = "" // the owner's own password is never kept in a readable form
	}
	var name, role string
	a.db.QueryRow(`SELECT name,role FROM staff_users WHERE id=? AND restaurant_id=?`, id, s.R.ID).Scan(&name, &role)
	// The owner sets their own password here too; nobody sets another owner's, and a manager
	// sets only kitchen, waiter and cashier passwords.
	if !s.CanManage(role) && !(id == s.ID && role == "owner") {
		back(w, r, "/owner/staff?err=The+password+could+not+be+changed.")
		return
	}
	err := a.inTx(func(q database.Querier) error {
		res, err := q.Exec(`UPDATE staff_users SET password_hash=?, password_enc=? WHERE id=? AND restaurant_id=? AND role=?`, string(hash), sealed, id, s.R.ID, role)
		if rows(res, err) != 1 {
			if err == nil {
				err = fmt.Errorf("staff %d not found", id)
			}
			return err
		}
		if id == s.ID { // the owner's own password: keep this browser logged in
			return nil
		}
		return exec(q, `DELETE FROM staff_sessions WHERE staff_id=?`, id)
	})
	if err != nil {
		back(w, r, "/owner/staff?err=The+password+could+not+be+changed.")
		return
	}
	a.audit(s.R.ID, s.Name, "password reset", name)
	back(w, r, "/owner/staff?ok=1")
}

func (a *App) settings(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "owner_settings", M{"Staff": s, "Page": "settings", "Saved": r.URL.Query().Get("saved") != "",
		"NetCount": len(splitNetworks(s.R.Networks))})
}

func (a *App) settingsSave(w http.ResponseWriter, r *http.Request, s *Staff) {
	pctOf := func(k string) float64 {
		f, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue(k)), 64)
		if err != nil || f < 0 || f > 100 {
			return 0
		}
		return f
	}
	// Times are "HH:MM" from <input type=time>; anything else is stored as empty (not shown).
	hm := func(k string) string {
		if m, ok := parseHM(r.FormValue(k)); ok {
			return fmt.Sprintf("%02d:%02d", m/60, m%60)
		}
		return ""
	}
	minutes := func(k string, fallback int) int {
		n, err := strconv.Atoi(strings.TrimSpace(r.FormValue(k)))
		if err != nil || n < 1 || n > 240 {
			return fallback
		}
		return n
	}
	warn := minutes("warn_minutes", 10)
	late := max(minutes("late_minutes", 20), warn+1)
	vat, svc := pctOf("vat"), pctOf("service")
	if !s.Can(PermCharges) { // a manager can't change VAT or service charge
		vat, svc = s.R.VAT, s.R.Service
	}
	if err := exec(a.db, `UPDATE restaurants SET vat_percent=?,service_percent=?,is_open=?,require_accept=?,open_time=?,close_time=?,last_order=?,warn_minutes=?,late_minutes=?,orders_in_hours=? WHERE id=?`,
		vat, svc, r.FormValue("is_open") == "1", r.FormValue("require_accept") == "1",
		hm("open_time"), hm("close_time"), hm("last_order"), warn, late, r.FormValue("orders_in_hours") == "1", s.R.ID); err != nil {
		serverError(w, err)
		return
	}
	if vat != s.R.VAT || svc != s.R.Service {
		a.audit(s.R.ID, s.Name, "charges changed", fmt.Sprintf("VAT %s%% → %s%%, service %s%% → %s%%", pct(s.R.VAT), pct(vat), pct(s.R.Service), pct(svc)))
	}
	back(w, r, "/owner/settings?saved=1")
}

// staffMove moves a person to another branch of the business and logs them out, so their next
// login opens the new branch. Owner only.
func (a *App) staffMove(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	var to int64
	fmt.Sscan(r.FormValue("branch"), &to)
	target := ""
	for _, b := range s.Branches {
		if b.ID == to {
			target = b.Label
		}
	}
	var name, role string
	var from int64
	err := a.db.QueryRow(`SELECT u.name,u.role,u.restaurant_id FROM staff_users u JOIN restaurants r ON r.id=u.restaurant_id
		WHERE u.id=? AND r.business_id=?`, id, s.R.BusinessID).Scan(&name, &role, &from)
	if err != nil || target == "" || role == "owner" {
		http.NotFound(w, r)
		return
	}
	err = a.inTx(func(q database.Querier) error {
		if err := exec(q, `UPDATE staff_users SET restaurant_id=? WHERE id=?`, to, id); err != nil {
			return err
		}
		return exec(q, `DELETE FROM staff_sessions WHERE staff_id=?`, id)
	})
	if err != nil {
		serverError(w, err)
		return
	}
	a.audit(from, s.Name, "staff moved", name+" → "+target)
	a.audit(to, s.Name, "staff moved", name+" → "+target)
	back(w, r, "/owner/staff")
}
