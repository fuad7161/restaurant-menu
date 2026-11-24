package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bosho/internal/database"
	"bosho/internal/i18n"

	"golang.org/x/crypto/bcrypt"
)

type Staff struct {
	ID                int64
	Name, Phone, Role string
	Email             string // owners only, for now
	EmailVerified     bool
	R                 Restaurant
	Lang              string // service screens: "en" or "bn" (see i18n.StaffLang)
}

func (a *App) staffRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		if a.goInside(w, r) {
			return
		}
		a.render(w, "login", M{"Next": safeNext(r.URL.Query().Get("next")), "Done": r.URL.Query().Get("done")})
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
	mux.HandleFunc("GET /staff/lang", a.need(PermAccount, a.staffLang))
	mux.HandleFunc("GET /staff/account", a.need(PermAccount, a.account))
	mux.HandleFunc("POST /staff/account/password", a.need(PermOwnPassword, a.accountPassword))
	mux.HandleFunc("POST /staff/account/logout-all", a.need(PermAccount, a.logoutAll))

	mux.HandleFunc("GET /staff/kitchen", a.need(PermKitchenView, a.kitchen))
	mux.HandleFunc("GET /staff/kitchen/board", a.poll(a.need(PermKitchenView, a.kitchenBoard)))
	// orderAct and itemAct check the permission of each action themselves (orderActPerm, itemActPerm).
	mux.HandleFunc("POST /staff/orders/{id}/{act}", a.auth(a.orderAct))
	mux.HandleFunc("POST /staff/items/{id}/{act}", a.auth(a.itemAct))
	mux.HandleFunc("POST /staff/calls/{id}/done", a.need(PermCallDone, a.callDone))
	mux.HandleFunc("GET /staff/stock", a.need(PermStock, a.stock))
	mux.HandleFunc("POST /staff/stock/{id}/toggle", a.need(PermStock, a.stockToggle))
	mux.HandleFunc("GET /staff/counter", a.need(PermCounterView, a.counter))
	mux.HandleFunc("GET /staff/counter/list", a.poll(a.need(PermCounterView, a.counterList)))
	mux.HandleFunc("GET /staff/sessions/{id}", a.need(PermBillView, a.sessionDetail))
	mux.HandleFunc("POST /staff/sessions/{id}/paid", a.need(PermBillPaid, a.sessionPaid))
	mux.HandleFunc("POST /staff/sessions/{id}/close", a.need(PermBillClose, a.sessionClose))
	mux.HandleFunc("POST /staff/sessions/{id}/discount", a.need(PermBillDiscount, a.sessionDiscount))
	mux.HandleFunc("GET /staff/sessions/{id}/print", a.need(PermBillView, a.sessionPrint))
	mux.HandleFunc("GET /staff/tables", a.need(PermTablesOrder, a.staffTables))
	mux.HandleFunc("POST /staff/tables/{id}/open", a.need(PermTablesOrder, a.staffOpenTable))
}

func (a *App) currentStaff(r *http.Request) *Staff {
	c, err := r.Cookie("bosho_staff")
	if err != nil {
		return nil
	}
	var s Staff
	err = a.db.QueryRow(`SELECT u.id,u.name,u.phone,u.role,COALESCE(u.email,''),u.email_verified_at>0,r.id,r.name,r.vat_percent,r.service_percent,r.is_open,r.require_accept,r.logo,r.brand_color,r.tagline,r.address,r.phone,r.order_networks,`+restExtraCols+`
		FROM staff_sessions ss JOIN staff_users u ON u.id=ss.staff_id JOIN restaurants r ON r.id=u.restaurant_id
		WHERE ss.token=? AND ss.expires_at>?`, c.Value, now()).
		Scan(append([]any{&s.ID, &s.Name, &s.Phone, &s.Role, &s.Email, &s.EmailVerified, &s.R.ID, &s.R.Name, &s.R.VAT, &s.R.Service, &s.R.IsOpen, &s.R.RequireAccept,
			&s.R.Logo, &s.R.Color, &s.R.Tagline, &s.R.Address, &s.R.Phone, &s.R.Networks}, s.R.extraDest()...)...)
	if err != nil {
		return nil
	}
	if !a.cfg.EmailVerify {
		// Email verification is switched off (EMAIL_VERIFY is not 1): treat every email as confirmed,
		// so no banner, no "Send it again" and no waiting. Nothing is written to the database.
		s.EmailVerified = true
	}
	s.Lang = i18n.StaffLang(r)
	return &s
}

// auth only checks that someone is logged in. Routes use need, which also checks the permission;
// auth alone is for routes that check a permission per action inside the handler.
func (a *App) auth(h func(http.ResponseWriter, *http.Request, *Staff)) http.HandlerFunc {
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
		h(w, r, s)
	}
}

func (a *App) startStaffSession(w http.ResponseWriter, r *http.Request, staffID int64) {
	tok := randToken(24)
	a.db.Exec(`DELETE FROM staff_sessions WHERE expires_at<?`, now())
	a.db.Exec(`INSERT INTO staff_sessions(token,staff_id,expires_at) VALUES(?,?,?)`, tok, staffID, now()+30*86400)
	setCookie(w, r, &http.Cookie{Name: "bosho_staff", Value: tok, MaxAge: 30 * 86400, HttpOnly: true})
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

// Wrong passwords: after loginTries for one phone number, or loginTriesNet from one network,
// logging in is refused for loginLock.
const (
	loginTries    = 5
	loginTriesNet = 30
	loginLock     = 15 * time.Minute
)

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	phone := clean(r.FormValue("phone"), 20)
	ip, _ := a.clientIP(r)
	phoneKey, ipKey := "login:"+phone, "loginip:"+ip.String()
	m := M{"Phone": phone, "Next": safeNext(r.FormValue("next"))}
	if a.limits.count(phoneKey, loginLock) >= loginTries || a.limits.count(ipKey, loginLock) >= loginTriesNet {
		slog.Warn("login locked", "phone", phone, "ip", ip.String())
		w.WriteHeader(http.StatusTooManyRequests)
		m["Err"] = "Too many wrong passwords. Wait 15 minutes and try again."
		a.render(w, "login", m)
		return
	}
	var id int64
	var hash, role string
	err := a.db.QueryRow(`SELECT id,password_hash,role FROM staff_users WHERE phone=?`, phone).Scan(&id, &hash, &role)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.FormValue("password"))) != nil {
		a.limits.hit(phoneKey, loginTries, loginLock)
		a.limits.hit(ipKey, loginTriesNet, loginLock)
		m["Err"] = "Wrong phone number or password."
		a.render(w, "login", m)
		return
	}
	a.limits.reset(phoneKey)
	a.startStaffSession(w, r, id)
	to := safeNext(r.FormValue("next"))
	if to == "" {
		to = staffHome(role)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (a *App) signup(w http.ResponseWriter, r *http.Request) {
	rest, name, phone, pass := clean(r.FormValue("restaurant"), 60), clean(r.FormValue("name"), 40), clean(r.FormValue("phone"), 20), r.FormValue("password")
	email := validEmail(r.FormValue("email"))
	m := M{"Restaurant": rest, "Name": name, "Phone": phone, "Email": r.FormValue("email")}
	if rest == "" || name == "" || len(phone) < 6 || len(pass) < 8 {
		m["Err"] = "Fill in every field. The password needs at least 8 characters."
		a.render(w, "signup", m)
		return
	}
	if email == "" {
		m["Err"] = "Enter a working email address. Bosho sends a link there if you forget your password."
		a.render(w, "signup", m)
		return
	}
	var taken bool
	a.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM staff_users WHERE lower(email)=lower(?))`, email).Scan(&taken)
	if taken {
		m["Err"] = "That email already has an account. Log in, or use \"Forgot password?\"."
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
	// A new owner starts a business with its first branch (docs/plans/multi-branch.md).
	bid, err := database.InsertID(tx, `INSERT INTO businesses(name,created_at) VALUES(?,?)`, rest, now())
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	rid, err := database.InsertID(tx, `INSERT INTO restaurants(business_id,name,slug,created_at) VALUES(?,?,?,?)`, bid, rest, randToken(6), now())
	if err != nil {
		http.Error(w, "database error", 500)
		return
	}
	sid, err := database.InsertID(tx, `INSERT INTO staff_users(restaurant_id,name,phone,email,password_hash,role) VALUES(?,?,?,?,?,'owner')`, rid, name, phone, email, string(hash))
	if err != nil {
		m["Err"] = "That phone number already has an account."
		a.render(w, "signup", m)
		return
	}
	if tx.Commit() != nil {
		http.Error(w, "database error", 500)
		return
	}
	if a.cfg.EmailVerify {
		a.limits.hit(fmt.Sprintf("verifymail:%d", sid), 1, time.Minute)
		a.limits.hit(fmt.Sprintf("verifymailday:%d", sid), 5, 24*time.Hour)
		a.sendLink(r, sid, name, email, "verify")
	}
	a.startStaffSession(w, r, sid)
	http.Redirect(w, r, "/owner", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("bosho_staff"); err == nil {
		a.db.Exec(`DELETE FROM staff_sessions WHERE token=?`, c.Value)
	}
	setCookie(w, r, &http.Cookie{Name: "bosho_staff", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// staffLang switches the service screens between English and Bangla and returns to the page.
func (a *App) staffLang(w http.ResponseWriter, r *http.Request, s *Staff) {
	l := "en"
	if r.URL.Query().Get("l") == "bn" {
		l = "bn"
	}
	setCookie(w, r, &http.Cookie{Name: "staff_lang", Value: l, MaxAge: 365 * 86400, HttpOnly: true})
	to := safeNext(r.URL.Query().Get("next"))
	if to == "" {
		to = safeNext(pageOf(r.Referer())) // back to the page the switch was pressed on
	}
	if to == "" || strings.HasPrefix(to, "/staff/lang") {
		to = staffHome(s.Role)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// account lets any staff member change their own password and log out on every device.
func (a *App) account(w http.ResponseWriter, r *http.Request, s *Staff) {
	q := r.URL.Query()
	a.render(w, "staff_account", M{"Staff": s, "Page": "account", "Err": q.Get("err"), "Done": q.Get("done")})
}

func (a *App) accountPassword(w http.ResponseWriter, r *http.Request, s *Staff) {
	var hash string
	a.db.QueryRow(`SELECT password_hash FROM staff_users WHERE id=?`, s.ID).Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.FormValue("current"))) != nil {
		back(w, r, "/staff/account?err=current")
		return
	}
	pass := r.FormValue("password")
	if len(pass) < 8 {
		back(w, r, "/staff/account?err=short")
		return
	}
	newHash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	// The new password ends every other login; this browser stays logged in.
	cur, _ := r.Cookie("bosho_staff")
	err := a.inTx(func(q database.Querier) error {
		if err := exec(q, `UPDATE staff_users SET password_hash=? WHERE id=?`, string(newHash), s.ID); err != nil {
			return err
		}
		return exec(q, `DELETE FROM staff_sessions WHERE staff_id=? AND token!=?`, s.ID, cur.Value)
	})
	if err != nil {
		serverError(w, err)
		return
	}
	a.audit(s.R.ID, s.Name, "password changed", s.Name)
	back(w, r, "/staff/account?done=password")
}

// logoutAll ends every login of this staff member, on every phone and tablet, including this one.
func (a *App) logoutAll(w http.ResponseWriter, r *http.Request, s *Staff) {
	if err := exec(a.db, `DELETE FROM staff_sessions WHERE staff_id=?`, s.ID); err != nil {
		serverError(w, err)
		return
	}
	setCookie(w, r, &http.Cookie{Name: "bosho_staff", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
