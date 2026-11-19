package server

import (
	"net/http"
	"net/url"
	"strings"

	"bosho/internal/database"

	"golang.org/x/crypto/bcrypt"
)

type Staff struct {
	ID                int64
	Name, Phone, Role string
	R                 Restaurant
}

func (s *Staff) IsOwner() bool { return s.Role == "owner" }

func (a *App) staffRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		if a.goInside(w, r) {
			return
		}
		a.render(w, "login", M{"Next": safeNext(r.URL.Query().Get("next"))})
	})
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("GET /signup", func(w http.ResponseWriter, r *http.Request) {
		if a.goInside(w, r) {
			return
		}
		a.render(w, "signup", M{})
	})
	mux.HandleFunc("POST /signup", a.signup)
	mux.HandleFunc("POST /logout", a.logout)

	mux.HandleFunc("GET /staff/kitchen", a.auth(false, a.kitchen))
	mux.HandleFunc("GET /staff/kitchen/board", a.poll(a.auth(false, a.kitchenBoard)))
	mux.HandleFunc("POST /staff/orders/{id}/{act}", a.auth(false, a.orderAct))
	mux.HandleFunc("POST /staff/items/{id}/{act}", a.auth(false, a.itemAct))
	mux.HandleFunc("POST /staff/calls/{id}/done", a.auth(false, a.callDone))
	mux.HandleFunc("GET /staff/stock", a.auth(false, a.stock))
	mux.HandleFunc("POST /staff/stock/{id}/toggle", a.auth(false, a.stockToggle))
	mux.HandleFunc("GET /staff/counter", a.auth(false, a.counter))
	mux.HandleFunc("GET /staff/counter/list", a.poll(a.auth(false, a.counterList)))
	mux.HandleFunc("GET /staff/sessions/{id}", a.auth(false, a.sessionDetail))
	mux.HandleFunc("POST /staff/sessions/{id}/paid", a.auth(false, a.sessionPaid))
	mux.HandleFunc("POST /staff/sessions/{id}/close", a.auth(false, a.sessionClose))
	mux.HandleFunc("GET /staff/tables", a.auth(false, a.staffTables))
	mux.HandleFunc("POST /staff/tables/{id}/open", a.auth(false, a.staffOpenTable))
}

func (a *App) currentStaff(r *http.Request) *Staff {
	c, err := r.Cookie("bosho_staff")
	if err != nil {
		return nil
	}
	var s Staff
	err = a.db.QueryRow(`SELECT u.id,u.name,u.phone,u.role,r.id,r.name,r.vat_percent,r.service_percent,r.is_open,r.require_accept,r.logo,r.brand_color,r.tagline,r.address,r.phone,r.order_networks
		FROM staff_sessions ss JOIN staff_users u ON u.id=ss.staff_id JOIN restaurants r ON r.id=u.restaurant_id
		WHERE ss.token=? AND ss.expires_at>?`, c.Value, now()).
		Scan(&s.ID, &s.Name, &s.Phone, &s.Role, &s.R.ID, &s.R.Name, &s.R.VAT, &s.R.Service, &s.R.IsOpen, &s.R.RequireAccept,
			&s.R.Logo, &s.R.Color, &s.R.Tagline, &s.R.Address, &s.R.Phone, &s.R.Networks)
	if err != nil {
		return nil
	}
	return &s
}

func (a *App) auth(ownerOnly bool, h func(http.ResponseWriter, *http.Request, *Staff)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := a.currentStaff(r)
		if s == nil {
			// Send them to log in, and back to this page afterwards. A refreshing fragment or a
			// form post comes back to the page it was on.
			if r.Header.Get("HX-Request") != "" {
				w.Header().Set("HX-Redirect", loginURL(pageOf(r.Header.Get("HX-Current-URL"))))
				return
			}
			next := r.URL.RequestURI()
			if r.Method != http.MethodGet {
				next = pageOf(r.Referer())
			}
			http.Redirect(w, r, loginURL(next), http.StatusSeeOther)
			return
		}
		if ownerOnly && !s.IsOwner() {
			http.Error(w, "Only the owner can open this page.", http.StatusForbidden)
			return
		}
		h(w, r, s)
	}
}

func (a *App) startStaffSession(w http.ResponseWriter, staffID int64) {
	tok := randToken(24)
	a.db.Exec(`DELETE FROM staff_sessions WHERE expires_at<?`, now())
	a.db.Exec(`INSERT INTO staff_sessions(token,staff_id,expires_at) VALUES(?,?,?)`, tok, staffID, now()+30*86400)
	http.SetCookie(w, &http.Cookie{Name: "bosho_staff", Value: tok, Path: "/", MaxAge: 30 * 86400, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

// goInside sends an already logged-in staff member from the login, signup or home page to the
// page they asked for (?next=) or their role's start page. It reports whether it redirected.
func (a *App) goInside(w http.ResponseWriter, r *http.Request) bool {
	s := a.currentStaff(r)
	if s == nil {
		return false
	}
	to := safeNext(r.URL.Query().Get("next"))
	if to == "" {
		to = staffHome(s.Role)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
	return true
}

func loginURL(next string) string {
	if next = safeNext(next); next == "" {
		return "/login"
	}
	return "/login?next=" + url.QueryEscape(next)
}

// pageOf turns a full URL from a header (Referer, HX-Current-URL) into a path on this site.
func pageOf(full string) string {
	u, err := url.Parse(full)
	if err != nil {
		return ""
	}
	return u.RequestURI()
}

// safeNext accepts only a path on this site, so ?next= cannot send someone to another website
// after logging in ("//evil.com" and "/\\evil.com" are other sites to a browser).
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") ||
		strings.ContainsAny(next, "\r\n") || next == "/" || strings.HasPrefix(next, "/login") ||
		strings.HasPrefix(next, "/signup") || strings.HasPrefix(next, "/logout") {
		return ""
	}
	return next
}

func staffHome(role string) string {
	switch role {
	case "owner":
		return "/owner"
	case "cashier":
		return "/staff/counter"
	}
	return "/staff/kitchen"
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	phone := clean(r.FormValue("phone"), 20)
	var id int64
	var hash, role string
	err := a.db.QueryRow(`SELECT id,password_hash,role FROM staff_users WHERE phone=?`, phone).Scan(&id, &hash, &role)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.FormValue("password"))) != nil {
		a.render(w, "login", M{"Err": "Wrong phone number or password.", "Phone": phone, "Next": safeNext(r.FormValue("next"))})
		return
	}
	a.startStaffSession(w, id)
	to := safeNext(r.FormValue("next"))
	if to == "" {
		to = staffHome(role)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (a *App) signup(w http.ResponseWriter, r *http.Request) {
	rest, name, phone, pass := clean(r.FormValue("restaurant"), 60), clean(r.FormValue("name"), 40), clean(r.FormValue("phone"), 20), r.FormValue("password")
	m := M{"Restaurant": rest, "Name": name, "Phone": phone}
	if rest == "" || name == "" || len(phone) < 6 || len(pass) < 8 {
		m["Err"] = "Fill in every field. The password needs at least 8 characters."
		a.render(w, "signup", m)
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	tx, err := a.db.Begin()
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	defer tx.Rollback()
	rid, err := database.InsertID(tx, `INSERT INTO restaurants(name,slug,created_at) VALUES(?,?,?)`, rest, randToken(6), now())
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	sid, err := database.InsertID(tx, `INSERT INTO staff_users(restaurant_id,name,phone,password_hash,role) VALUES(?,?,?,?,'owner')`, rid, name, phone, string(hash))
	if err != nil {
		m["Err"] = "That phone number already has an account."
		a.render(w, "signup", m)
		return
	}
	if tx.Commit() != nil {
		http.Error(w, "database error", 500)
		return
	}
	a.startStaffSession(w, sid)
	http.Redirect(w, r, "/owner", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("bosho_staff"); err == nil {
		a.db.Exec(`DELETE FROM staff_sessions WHERE token=?`, c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "bosho_staff", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
