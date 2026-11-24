package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"bosho/internal/database"
	bmail "bosho/internal/mail"
)

// The platform admin panel, first part (docs/plans/multi-branch.md, step 7): admins log in with
// email, password and a 6-digit code sent to that email, list the businesses, and set each
// business's branch limit and email verification. Admin logins are separate from restaurant
// logins: their own table, cookie and session, so a restaurant login never opens /admin.

const (
	adminSessionTTL = 12 * time.Hour
	adminCodeTTL    = 10 * time.Minute
	adminCodeTries  = 5
	maxBranchLimit  = 100
)

type Admin struct {
	ID    int64
	Email string
}

func (a *App) adminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/login", func(w http.ResponseWriter, r *http.Request) {
		if a.currentAdmin(r) != nil {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		a.render(w, "admin_login", M{})
	})
	mux.HandleFunc("POST /admin/login", a.adminLogin)
	mux.HandleFunc("GET /admin/code", func(w http.ResponseWriter, r *http.Request) { a.render(w, "admin_code", M{}) })
	mux.HandleFunc("POST /admin/code", a.adminCode)
	mux.HandleFunc("POST /admin/logout", a.adminLogout)
	mux.HandleFunc("GET /admin", a.admin(a.adminHome))
	mux.HandleFunc("GET /admin/businesses/{id}", a.admin(a.adminBusiness))
	mux.HandleFunc("POST /admin/businesses/{id}/branches", a.admin(a.adminSetBranches))
	mux.HandleFunc("POST /admin/businesses/{id}/verify", a.admin(a.adminSetVerify))
}

// CreateAdmin adds a platform admin, or sets a new password for an existing one. It is run from
// the command line (bosho create-admin), never from the web.
func CreateAdmin(db *database.DB, email, password string) error {
	if validEmail(email) == "" {
		return errors.New("not an email address")
	}
	if len(password) < 12 {
		return errors.New("the password needs at least 12 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := db.Exec(`UPDATE admins SET password_hash=? WHERE lower(email)=lower(?)`, string(hash), email)
	if rows(res, err) == 1 {
		_, err = db.Exec(`DELETE FROM admin_sessions WHERE admin_id=(SELECT id FROM admins WHERE lower(email)=lower(?))`, email)
		return err
	}
	_, err = db.Exec(`INSERT INTO admins(email,password_hash,created_at) VALUES(?,?,?)`, email, string(hash), now())
	return err
}

func (a *App) currentAdmin(r *http.Request) *Admin {
	c, err := r.Cookie("bosho_admin")
	if err != nil {
		return nil
	}
	var ad Admin
	if a.db.QueryRow(`SELECT a.id,a.email FROM admin_sessions s JOIN admins a ON a.id=s.admin_id WHERE s.token=? AND s.expires_at>?`,
		tokenHash(c.Value), now()).Scan(&ad.ID, &ad.Email) != nil {
		return nil
	}
	return &ad
}

// admin wraps an admin page: without an admin login it goes to /admin/login.
func (a *App) admin(h func(http.ResponseWriter, *http.Request, *Admin)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ad := a.currentAdmin(r)
		if ad == nil {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		h(w, r, ad)
	}
}

func (a *App) adminAudit(ad *Admin, bid int64, action, detail string) {
	a.db.Exec(`INSERT INTO admin_audit(admin_id,business_id,action,detail,created_at) VALUES(?,?,?,?,?)`, ad.ID, bid, action, clean(detail, 400), now())
}

// adminLogin checks the email and password, then emails a 6-digit code. Wrong passwords count
// per email and per network, like the staff login.
func (a *App) adminLogin(w http.ResponseWriter, r *http.Request) {
	email := clean(r.FormValue("email"), 120)
	ip, _ := a.clientIP(r)
	emailKey, ipKey := "adminlogin:"+strings.ToLower(email), "adminloginip:"+ip.String()
	m := M{"Email": email}
	if a.limits.count(emailKey, loginLock) >= loginTries || a.limits.count(ipKey, loginLock) >= loginTries*2 {
		slog.Warn("admin login locked", "email", email, "ip", ip.String())
		w.WriteHeader(http.StatusTooManyRequests)
		m["Err"] = "Too many wrong tries. Wait 15 minutes and try again."
		a.render(w, "admin_login", m)
		return
	}
	var id int64
	var hash, to string
	err := a.db.QueryRow(`SELECT id,password_hash,email FROM admins WHERE lower(email)=lower(?)`, email).Scan(&id, &hash, &to)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.FormValue("password"))) != nil {
		a.limits.hit(emailKey, loginTries, loginLock)
		a.limits.hit(ipKey, loginTries*2, loginLock)
		slog.Warn("admin login failed", "email", email, "ip", ip.String())
		m["Err"] = "Wrong email or password."
		a.render(w, "admin_login", m)
		return
	}
	a.limits.reset(emailKey)
	code := sixDigits()
	pending := randToken(24)
	err = a.inTx(func(q database.Querier) error {
		if err := exec(q, `DELETE FROM admin_codes WHERE admin_id=? OR expires_at<?`, id, now()); err != nil {
			return err
		}
		return exec(q, `INSERT INTO admin_codes(pending,admin_id,code_hash,expires_at) VALUES(?,?,?,?)`,
			tokenHash(pending), id, tokenHash(code), now()+int64(adminCodeTTL.Seconds()))
	})
	if err != nil {
		serverError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.mail.Send(ctx, adminCodeEmail(to, code)); err != nil {
		slog.Error("admin code not sent", "admin", id, "err", err)
		m["Err"] = "The code could not be emailed. Try again in a minute."
		a.render(w, "admin_login", m)
		return
	}
	setCookie(w, r, &http.Cookie{Name: "bosho_admin_pending", Value: pending, MaxAge: int(adminCodeTTL.Seconds()), HttpOnly: true})
	http.Redirect(w, r, "/admin/code", http.StatusSeeOther)
}

// adminCode checks the emailed code. Five wrong codes end that try; the admin starts over.
func (a *App) adminCode(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("bosho_admin_pending")
	if err != nil {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	var id int64
	var codeHash string
	var tries int
	err = a.db.QueryRow(`SELECT admin_id,code_hash,tries FROM admin_codes WHERE pending=? AND expires_at>?`, tokenHash(c.Value), now()).Scan(&id, &codeHash, &tries)
	if err != nil || tries >= adminCodeTries {
		a.render(w, "admin_login", M{"Err": "The code has expired. Log in again for a new one."})
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	if subtle.ConstantTimeCompare([]byte(tokenHash(code)), []byte(codeHash)) != 1 {
		a.db.Exec(`UPDATE admin_codes SET tries=tries+1 WHERE pending=?`, tokenHash(c.Value))
		a.render(w, "admin_code", M{"Err": "That code is not right. Check the newest email from Bosho."})
		return
	}
	tok := randToken(24)
	err = a.inTx(func(q database.Querier) error {
		if err := exec(q, `DELETE FROM admin_codes WHERE admin_id=?`, id); err != nil {
			return err
		}
		exec(q, `DELETE FROM admin_sessions WHERE expires_at<?`, now())
		return exec(q, `INSERT INTO admin_sessions(token,admin_id,expires_at) VALUES(?,?,?)`, tokenHash(tok), id, now()+int64(adminSessionTTL.Seconds()))
	})
	if err != nil {
		serverError(w, err)
		return
	}
	setCookie(w, r, &http.Cookie{Name: "bosho_admin_pending", MaxAge: -1, HttpOnly: true})
	setCookie(w, r, &http.Cookie{Name: "bosho_admin", Value: tok, MaxAge: int(adminSessionTTL.Seconds()), HttpOnly: true})
	slog.Info("admin logged in", "admin", id)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (a *App) adminLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("bosho_admin"); err == nil {
		a.db.Exec(`DELETE FROM admin_sessions WHERE token=?`, tokenHash(c.Value))
	}
	setCookie(w, r, &http.Cookie{Name: "bosho_admin", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func sixDigits() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func adminCodeEmail(to, code string) bmail.Message {
	text := fmt.Sprintf("Your Bosho admin login code is %s\n\nIt works once and expires in 10 minutes. If you did not just log in, change your admin password.\n", code)
	body := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:480px;margin:auto;padding:24px;color:#1d1d1f">
<p style="font-size:20px;font-weight:700;margin:0 0 16px">Bosho admin</p>
<p>Your login code:</p><p style="font-size:32px;font-weight:700;letter-spacing:6px">%s</p>
<p style="color:#666;font-size:14px">It works once and expires in 10 minutes. If you did not just log in, change your admin password.</p></div>`, html.EscapeString(code))
	return bmail.Message{To: to, Subject: "Bosho admin login code", HTML: body, Text: text}
}

type adminBizRow struct {
	ID                        int64
	Name, Owner, Phone, Email string
	Verified                  bool
	Branches, Max             int
	Created                   int64
	OrdersToday, Orders       int
	Verify                    string // global | required | off, or "mixed" when its branches differ
}

// adminBizSQL lists businesses with their owner and counts; add a WHERE on b and the arguments.
const adminBizSQL = `SELECT b.id,b.name,COALESCE(o.name,''),COALESCE(o.phone,''),COALESCE(o.email,''),COALESCE(o.email_verified_at>0,false),
	(SELECT COUNT(*) FROM restaurants r WHERE r.business_id=b.id AND r.status!='archived'),b.max_branches,b.created_at,
	(SELECT COUNT(*) FROM orders x JOIN restaurants r ON r.id=x.restaurant_id WHERE r.business_id=b.id AND x.placed_at>=?),
	(SELECT COUNT(*) FROM orders x JOIN restaurants r ON r.id=x.restaurant_id WHERE r.business_id=b.id),
	(SELECT CASE WHEN COUNT(DISTINCT r.verify_email)=1 THEN MIN(r.verify_email) ELSE 'mixed' END FROM restaurants r WHERE r.business_id=b.id)
	FROM businesses b LEFT JOIN LATERAL (SELECT u.name,u.phone,u.email,u.email_verified_at FROM staff_users u JOIN restaurants r ON r.id=u.restaurant_id
		WHERE r.business_id=b.id AND u.role='owner' ORDER BY u.id LIMIT 1) o ON true`

func scanAdminBiz(rs *sql.Rows, x *adminBizRow) error {
	return rs.Scan(&x.ID, &x.Name, &x.Owner, &x.Phone, &x.Email, &x.Verified, &x.Branches, &x.Max, &x.Created, &x.OrdersToday, &x.Orders, &x.Verify)
}

// adminHome lists every business, newest first, optionally searched by name, owner, phone or email.
func (a *App) adminHome(w http.ResponseWriter, r *http.Request, ad *Admin) {
	q := clean(r.URL.Query().Get("q"), 60)
	where, args := "", []any{a.dayStart()}
	if q != "" {
		where = ` WHERE b.name ILIKE ? OR o.name ILIKE ? OR o.phone LIKE ? OR o.email ILIKE ?`
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		args = append(args, like, like, like, like)
	}
	rows := database.List(a.db, adminBizSQL+where+` ORDER BY b.id DESC LIMIT 500`, scanAdminBiz, args...)
	var businesses, branches int
	a.db.QueryRow(`SELECT (SELECT COUNT(*) FROM businesses),(SELECT COUNT(*) FROM restaurants WHERE status!='archived')`).Scan(&businesses, &branches)
	a.render(w, "admin_home", M{"Admin": ad, "Rows": rows, "Q": q, "Businesses": businesses, "BranchCount": branches, "EmailVerify": a.cfg.EmailVerify})
}

type adminAuditRow struct {
	Who, Action, Detail string
	At                  int64
}

func (a *App) adminBusiness(w http.ResponseWriter, r *http.Request, ad *Admin) {
	id := pathID(r, "id")
	biz := database.List(a.db, adminBizSQL+` WHERE b.id=?`, scanAdminBiz, a.dayStart(), id)
	if len(biz) == 0 {
		http.NotFound(w, r)
		return
	}
	branches := database.List(a.db, `SELECT r.id,CASE WHEN r.branch_name!='' THEN r.branch_name ELSE r.name END,r.branch_code,r.status FROM restaurants r
		WHERE r.business_id=? ORDER BY r.sort_order,r.id`,
		func(rs *sql.Rows, b *Branch) error { return rs.Scan(&b.ID, &b.Label, &b.Code, &b.Status) }, id)
	log := database.List(a.db, `SELECT a.email,l.action,l.detail,l.created_at FROM admin_audit l JOIN admins a ON a.id=l.admin_id
		WHERE l.business_id=? ORDER BY l.id DESC LIMIT 50`,
		func(rs *sql.Rows, x *adminAuditRow) error { return rs.Scan(&x.Who, &x.Action, &x.Detail, &x.At) }, id)
	q := r.URL.Query()
	a.render(w, "admin_business", M{"Admin": ad, "B": biz[0], "Branches": branches, "Log": log, "EmailVerify": a.cfg.EmailVerify,
		"Saved": q.Get("saved") != "", "Err": q.Get("err")})
}

// adminSetBranches sets how many branches (not archived) the business may have. A lower limit
// never closes branches that are already in use; it only stops new ones.
func (a *App) adminSetBranches(w http.ResponseWriter, r *http.Request, ad *Admin) {
	id := pathID(r, "id")
	page := fmt.Sprintf("/admin/businesses/%d", id)
	var n int
	if _, err := fmt.Sscan(r.FormValue("max_branches"), &n); err != nil || n < 1 || n > maxBranchLimit {
		http.Redirect(w, r, page+"?err="+urlQuery(fmt.Sprintf("The limit must be a number from 1 to %d.", maxBranchLimit)), http.StatusSeeOther)
		return
	}
	var old int
	if a.db.QueryRow(`SELECT max_branches FROM businesses WHERE id=?`, id).Scan(&old) != nil {
		http.NotFound(w, r)
		return
	}
	if old != n {
		if err := exec(a.db, `UPDATE businesses SET max_branches=? WHERE id=?`, n, id); err != nil {
			serverError(w, err)
			return
		}
		a.adminAudit(ad, id, "branch limit", fmt.Sprintf("%d → %d", old, n))
	}
	http.Redirect(w, r, page+"?saved=1", http.StatusSeeOther)
}

// adminSetVerify sets email verification for every branch of the business: global (follow
// EMAIL_VERIFY), required or off.
func (a *App) adminSetVerify(w http.ResponseWriter, r *http.Request, ad *Admin) {
	id, v := pathID(r, "id"), r.FormValue("verify")
	if v != "global" && v != "required" && v != "off" {
		http.Error(w, "unknown setting", http.StatusBadRequest)
		return
	}
	var old string
	if a.db.QueryRow(`SELECT CASE WHEN COUNT(DISTINCT verify_email)=1 THEN MIN(verify_email) ELSE 'mixed' END FROM restaurants WHERE business_id=? HAVING COUNT(*)>0`, id).Scan(&old) != nil {
		http.NotFound(w, r)
		return
	}
	if old != v {
		if err := exec(a.db, `UPDATE restaurants SET verify_email=? WHERE business_id=?`, v, id); err != nil {
			serverError(w, err)
			return
		}
		a.adminAudit(ad, id, "email verification", old+" → "+v)
	}
	http.Redirect(w, r, fmt.Sprintf("/admin/businesses/%d?saved=1", id), http.StatusSeeOther)
}
