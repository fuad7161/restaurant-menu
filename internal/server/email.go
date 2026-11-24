package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"bosho/internal/database"
	bmail "bosho/internal/mail"
)

// Owner email: a link to confirm the address, and a link to set a new password when the owner
// forgot theirs. Staff passwords are set (and can be looked up) by the owner instead.
const (
	verifyTTL = 24 * time.Hour
	resetTTL  = time.Hour
)

func (a *App) emailRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /forgot", func(w http.ResponseWriter, r *http.Request) { a.render(w, "forgot", M{}) })
	mux.HandleFunc("POST /forgot", a.forgot)
	mux.HandleFunc("GET /reset", a.resetForm)
	mux.HandleFunc("POST /reset", a.resetSave)
	mux.HandleFunc("GET /verify-email", a.verifyEmail)
	mux.HandleFunc("POST /staff/account/email", a.need(PermOwnEmail, a.accountEmail))
	mux.HandleFunc("POST /staff/account/email/resend", a.need(PermOwnEmail, a.resendVerify))
}

// awaitingEmail reports whether this restaurant must wait for its owner to confirm their email
// before guests can order: the platform setting (EMAIL_VERIFY) or the restaurant's own override.
// EMAIL_VERIFY is the master switch: while it is off, no restaurant waits, even one set to "required".
func (a *App) awaitingEmail(r Restaurant) bool {
	if !a.cfg.EmailVerify {
		return false
	}
	return r.VerifyEmail != "off" && !r.OwnerVerified
}

// validEmail returns the address in a normal form, or "" when it isn't one.
func validEmail(in string) string {
	in = strings.TrimSpace(in)
	addr, err := mail.ParseAddress(in)
	if err != nil || addr.Address != in || len(in) > 120 || !strings.Contains(in[strings.LastIndex(in, "@"):], ".") {
		return ""
	}
	return in
}

// sendLink stores a new single-use token and emails its link. purpose is "verify" or "reset_password".
func (a *App) sendLink(r *http.Request, staffID int64, name, email, purpose string) error {
	tok := randToken(24)
	ttl, path := verifyTTL, "/verify-email"
	if purpose == "reset_password" {
		ttl, path = resetTTL, "/reset"
	}
	// A new link replaces the older ones of the same kind.
	if err := a.inTx(func(q database.Querier) error {
		if err := exec(q, `DELETE FROM email_tokens WHERE staff_id=? AND purpose=? AND used_at=0`, staffID, purpose); err != nil {
			return err
		}
		return exec(q, `INSERT INTO email_tokens(token_hash,staff_id,purpose,email,created_at,expires_at) VALUES(?,?,?,?,?,?)`,
			tokenHash(tok), staffID, purpose, email, now(), now()+int64(ttl.Seconds()))
	}); err != nil {
		return err
	}
	link := a.base(r) + path + "?token=" + tok
	m := linkEmail(purpose, name, email, link)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.mail.Send(ctx, m); err != nil {
		slog.Error("email not sent", "purpose", purpose, "staff", staffID, "err", err)
		return err
	}
	return nil
}

// linkEmail writes the verify or reset message, in HTML and plain text.
func linkEmail(purpose, name, to, link string) bmail.Message {
	subject, intro, button, after := "Confirm your email for Bosho",
		"Please confirm this is your email address. Bosho uses it to send you a link if you ever forget your password.",
		"Confirm email", "This link works once and expires in 24 hours. If you didn't sign up for Bosho, ignore this email."
	if purpose == "reset_password" {
		subject, intro, button, after = "Set a new Bosho password",
			"Someone asked to set a new password for your Bosho account. If it was you, use the button below.",
			"Set a new password", "This link works once and expires in 1 hour. Using it logs you out on every device. If you didn't ask for this, ignore this email: your password stays the same."
	}
	h := html.EscapeString
	body := fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:480px;margin:auto;padding:24px;color:#1d1d1f">
<p style="font-size:20px;font-weight:700;margin:0 0 16px">Bosho</p>
<p>Hello %s,</p><p>%s</p>
<p style="margin:24px 0"><a href="%s" style="background:#1f6f4a;color:#fff;padding:12px 20px;border-radius:8px;text-decoration:none;font-weight:600">%s</a></p>
<p style="color:#666;font-size:14px">%s</p>
<p style="color:#666;font-size:13px">If the button doesn't work, open this link:<br><a href="%s">%s</a></p></div>`,
		h(name), h(intro), h(link), h(button), h(after), h(link), h(link))
	text := fmt.Sprintf("Hello %s,\n\n%s\n\n%s: %s\n\n%s\n", name, intro, button, link, after)
	return bmail.Message{To: to, Subject: subject, HTML: body, Text: text}
}

// forgot sends a reset link to the owner's email. The answer is the same whether or not the
// account exists, so the page can't be used to find out who has an account.
func (a *App) forgot(w http.ResponseWriter, r *http.Request) {
	who := clean(r.FormValue("who"), 120)
	ip, _ := a.clientIP(r)
	done := M{"Sent": true}
	if who == "" {
		a.render(w, "forgot", M{"Err": "Enter your phone number or email."})
		return
	}
	if !a.limits.hit("forgot:"+strings.ToLower(who), 3, time.Hour) || !a.limits.hit("forgotip:"+ip.String(), 20, time.Hour) {
		a.render(w, "forgot", done)
		return
	}
	var id int64
	var name, email, role string
	err := a.db.QueryRow(`SELECT id,name,COALESCE(email,''),role FROM staff_users WHERE phone=? OR lower(email)=lower(?) ORDER BY id LIMIT 1`, who, who).
		Scan(&id, &name, &email, &role)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		slog.Error("forgot password", "err", err)
	case role != "owner" || email == "":
		// Staff passwords are set by the owner; an owner without an email asks the platform admin.
		slog.Info("forgot password: no email link possible", "staff", id, "role", role)
	default:
		a.sendLink(r, id, name, email, "reset_password")
	}
	a.render(w, "forgot", done)
}

// resetToken finds a usable token of the given purpose.
func (a *App) resetToken(tok string) (staffID int64, ok bool) {
	err := a.db.QueryRow(`SELECT staff_id FROM email_tokens WHERE token_hash=? AND purpose='reset_password' AND used_at=0 AND expires_at>?`,
		tokenHash(tok), now()).Scan(&staffID)
	return staffID, err == nil
}

func (a *App) resetForm(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	_, ok := a.resetToken(tok)
	a.render(w, "reset", M{"Token": tok, "Bad": !ok})
}

func (a *App) resetSave(w http.ResponseWriter, r *http.Request) {
	tok, pass := r.FormValue("token"), r.FormValue("password")
	if len(pass) < 8 {
		a.render(w, "reset", M{"Token": tok, "Err": "The password needs at least 8 characters."})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	var staffID, rid int64
	var name string
	err := a.inTx(func(q database.Querier) error {
		var email string
		if err := q.QueryRow(`UPDATE email_tokens SET used_at=? WHERE token_hash=? AND purpose='reset_password' AND used_at=0 AND expires_at>? RETURNING staff_id,email`,
			now(), tokenHash(tok), now()).Scan(&staffID, &email); err != nil {
			return err
		}
		// Opening the link proves the owner reads that inbox, so the address counts as confirmed.
		if err := q.QueryRow(`UPDATE staff_users SET password_hash=?, email_verified_at=CASE WHEN lower(email)=lower(?) AND email_verified_at=0 THEN ? ELSE email_verified_at END
			WHERE id=? RETURNING restaurant_id,name`, string(hash), email, now(), staffID).Scan(&rid, &name); err != nil {
			return err
		}
		if err := exec(q, `DELETE FROM email_tokens WHERE staff_id=? AND purpose='reset_password' AND used_at=0`, staffID); err != nil {
			return err
		}
		return exec(q, `DELETE FROM staff_sessions WHERE staff_id=?`, staffID)
	})
	if errors.Is(err, sql.ErrNoRows) {
		a.render(w, "reset", M{"Bad": true})
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	a.audit(rid, name, "password reset by email", name)
	http.Redirect(w, r, "/login?done=reset", http.StatusSeeOther)
}

// verifyEmail confirms the address the link was sent to, if it is still the account's email.
func (a *App) verifyEmail(w http.ResponseWriter, r *http.Request) {
	tok := r.URL.Query().Get("token")
	var rid int64
	var name string
	err := a.inTx(func(q database.Querier) error {
		var staffID int64
		var email string
		if err := q.QueryRow(`UPDATE email_tokens SET used_at=? WHERE token_hash=? AND purpose='verify' AND used_at=0 AND expires_at>? RETURNING staff_id,email`,
			now(), tokenHash(tok), now()).Scan(&staffID, &email); err != nil {
			return err
		}
		return q.QueryRow(`UPDATE staff_users SET email_verified_at=? WHERE id=? AND lower(email)=lower(?) RETURNING restaurant_id,name`,
			now(), staffID, email).Scan(&rid, &name)
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		serverError(w, err)
		return
	}
	if err == nil {
		a.audit(rid, name, "email confirmed", name)
	}
	a.render(w, "verified", M{"OK": err == nil})
}

// accountEmail sets or changes the owner's email. The current password is asked for, because the
// email is how the account gets back in. A new address must be confirmed again.
func (a *App) accountEmail(w http.ResponseWriter, r *http.Request, s *Staff) {
	email := validEmail(r.FormValue("email"))
	if email == "" {
		back(w, r, "/staff/account?err=email")
		return
	}
	var hash string
	a.db.QueryRow(`SELECT password_hash FROM staff_users WHERE id=?`, s.ID).Scan(&hash)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.FormValue("current"))) != nil {
		back(w, r, "/staff/account?err=current")
		return
	}
	if strings.EqualFold(email, s.Email) {
		back(w, r, "/staff/account")
		return
	}
	if _, err := a.db.Exec(`UPDATE staff_users SET email=?, email_verified_at=0 WHERE id=?`, email, s.ID); err != nil {
		back(w, r, "/staff/account?err=email_taken")
		return
	}
	a.audit(s.R.ID, s.Name, "email changed", email)
	if !a.cfg.EmailVerify {
		back(w, r, "/staff/account")
		return
	}
	a.limits.hit("verifymail:"+fmt.Sprint(s.ID), 1, time.Minute)
	a.limits.hit("verifymailday:"+fmt.Sprint(s.ID), 5, 24*time.Hour)
	if a.sendLink(r, s.ID, s.Name, email, "verify") != nil {
		back(w, r, "/staff/account?err=send")
		return
	}
	back(w, r, "/staff/account?done=email")
}

// resendVerify sends the confirmation link again: at most once a minute and 5 times a day.
func (a *App) resendVerify(w http.ResponseWriter, r *http.Request, s *Staff) {
	if s.Email == "" || s.EmailVerified {
		back(w, r, "/staff/account")
		return
	}
	id := fmt.Sprint(s.ID)
	if a.limits.count("verifymail:"+id, time.Minute) >= 1 || a.limits.count("verifymailday:"+id, 24*time.Hour) >= 5 {
		back(w, r, "/staff/account?err=wait")
		return
	}
	a.limits.hit("verifymail:"+id, 1, time.Minute)
	a.limits.hit("verifymailday:"+id, 5, 24*time.Hour)
	if a.sendLink(r, s.ID, s.Name, s.Email, "verify") != nil {
		back(w, r, "/staff/account?err=send")
		return
	}
	back(w, r, "/staff/account?done=sent")
}
