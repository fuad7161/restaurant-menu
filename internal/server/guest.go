package server

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"bosho/internal/database"
	"bosho/internal/i18n"
)

const (
	sessionIdle  = 2 * 3600 // an open session with no orders expires after this many idle seconds
	receiptHours = 3 * 3600 // a paid guest keeps seeing the receipt this long
	orderGap     = 10       // minimum seconds between two orders from one group
)

func (a *App) guestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { a.render(w, "home", nil) })
	mux.HandleFunc("GET /t/{tok}", a.gLanding)
	mux.HandleFunc("POST /t/{tok}/enter", a.gEnter)
	mux.HandleFunc("GET /t/{tok}/wait", a.gWait)
	mux.HandleFunc("POST /t/{tok}/leave", a.gLeave)
	mux.HandleFunc("GET /t/{tok}/lang", a.gLang)
	mux.HandleFunc("GET /t/{tok}/bar", a.gBar)
	mux.HandleFunc("POST /t/{tok}/join/{gid}/{act}", a.gJoinDecide)
	mux.HandleFunc("GET /t/{tok}/add/{id}", a.gAddControl)
	mux.HandleFunc("POST /t/{tok}/cart/add", a.gCartAdd)
	mux.HandleFunc("GET /t/{tok}/cart", a.gCart)
	mux.HandleFunc("GET /t/{tok}/cart/list", a.gCartList)
	mux.HandleFunc("POST /t/{tok}/cart/{id}/qty", a.gCartQty)
	mux.HandleFunc("POST /t/{tok}/cart/{id}/note", a.gCartNote)
	mux.HandleFunc("POST /t/{tok}/order", a.gPlaceOrder)
	mux.HandleFunc("GET /t/{tok}/orders", a.gOrders)
	mux.HandleFunc("GET /t/{tok}/orders/live", a.gOrdersLive)
	mux.HandleFunc("POST /t/{tok}/items/{id}/change", a.gItemChange)
	mux.HandleFunc("POST /t/{tok}/call", a.gCall)
	mux.HandleFunc("POST /t/{tok}/bill", a.gBill)
	mux.HandleFunc("POST /t/{tok}/bill/cancel", a.gBill)
	mux.HandleFunc("POST /t/{tok}/review", a.gReview)
}

// gctx is everything known about the phone that made the request.
type gctx struct {
	Lang, Tok, Dev string
	R              Restaurant
	Tbl            Table
	S              *Session
	G              *Guest
}

func (c *gctx) data() M {
	return M{"Lang": c.Lang, "Tok": c.Tok, "R": c.R, "Tbl": c.Tbl, "S": c.S, "G": c.G}
}

func (c *gctx) url(path string) string { return "/t/" + c.Tok + path }

func deviceToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("bosho_dev"); err == nil && len(c.Value) >= 16 {
		return c.Value
	}
	tok := randToken(16)
	http.SetCookie(w, &http.Cookie{Name: "bosho_dev", Value: tok, Path: "/", MaxAge: 365 * 86400, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	return tok
}

func (a *App) gctx(w http.ResponseWriter, r *http.Request) (*gctx, bool) {
	c := &gctx{Tok: r.PathValue("tok"), Lang: i18n.Lang(r)}
	err := a.db.QueryRow(`SELECT t.id,t.label,r.id,r.name,r.vat_percent,r.service_percent,r.is_open,r.require_accept
		FROM tables t JOIN restaurants r ON r.id=t.restaurant_id WHERE t.qr_token=? AND t.is_active`, c.Tok).
		Scan(&c.Tbl.ID, &c.Tbl.Label, &c.R.ID, &c.R.Name, &c.R.VAT, &c.R.Service, &c.R.IsOpen, &c.R.RequireAccept)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	c.Dev = deviceToken(w, r)
	// Expire abandoned sessions that never ordered anything.
	a.db.Exec(`UPDATE table_sessions SET status='closed', closed_at=? WHERE table_id=? AND status='open' AND last_activity<?
		AND NOT EXISTS (SELECT 1 FROM orders o WHERE o.session_id=table_sessions.id AND o.status NOT IN ('rejected','cancelled'))`,
		now(), c.Tbl.ID, now()-sessionIdle)

	var s Session
	var g Guest
	err = a.db.QueryRow(`SELECT g.id,g.display_name,g.status,g.is_staff,s.id,s.group_label,s.status,s.opened_at,s.closed_at
		FROM session_guests g JOIN table_sessions s ON s.id=g.session_id
		WHERE s.table_id=? AND g.device_token=? AND g.status IN ('pending','joined')
		  AND (s.status IN ('open','bill_requested') OR (s.status='paid' AND s.closed_at>?))
		ORDER BY g.last_seen DESC, g.id DESC LIMIT 1`, c.Tbl.ID, c.Dev, now()-receiptHours).
		Scan(&g.ID, &g.Name, &g.Status, &g.IsStaff, &s.ID, &s.Label, &s.Status, &s.OpenedAt, &s.ClosedAt)
	if err == nil {
		c.S, c.G = &s, &g
	}
	return c, true
}

// joined returns the context only for a guest who is an approved member of a session.
func (a *App) joined(w http.ResponseWriter, r *http.Request) (*gctx, bool) {
	c, ok := a.gctx(w, r)
	if !ok {
		return nil, false
	}
	if c.G == nil || c.G.Status != "joined" {
		if r.Header.Get("HX-Request") != "" {
			w.Header().Set("HX-Redirect", c.url(""))
		} else {
			http.Redirect(w, r, c.url(""), http.StatusSeeOther)
		}
		return nil, false
	}
	return c, true
}

type groupRow struct {
	ID    int64
	Label string
	First string
}

func (a *App) openGroups(tableID int64) []groupRow {
	return database.List(a.db, `SELECT s.id,s.group_label,COALESCE((SELECT display_name FROM session_guests g
		WHERE g.session_id=s.id AND g.status='joined' ORDER BY g.id LIMIT 1),'')
		FROM table_sessions s WHERE s.table_id=? AND s.status IN ('open','bill_requested') ORDER BY s.id`,
		func(r *sql.Rows, g *groupRow) error { return r.Scan(&g.ID, &g.Label, &g.First) }, tableID)
}

func (a *App) nextLabel(tableID int64) string {
	used := map[string]bool{}
	for _, g := range a.openGroups(tableID) {
		used[g.Label] = true
	}
	for i := 0; i < 26; i++ {
		if l := string(rune('A' + i)); !used[l] {
			return l
		}
	}
	return "Z"
}

func (a *App) gLanding(w http.ResponseWriter, r *http.Request) {
	c, ok := a.gctx(w, r)
	if !ok {
		return
	}
	m := c.data()
	switch {
	case c.G == nil:
		m["Groups"] = a.openGroups(c.Tbl.ID)
		a.render(w, "guest_enter", m)
	case c.G.Status == "pending":
		a.render(w, "guest_wait", m)
	case c.S.Status == "paid":
		http.Redirect(w, r, c.url("/orders"), http.StatusSeeOther)
	default:
		a.barData(c, m, "menu")
		m["Cats"] = a.menu(c.R.ID)
		a.render(w, "guest_menu", m)
	}
}

func (a *App) gEnter(w http.ResponseWriter, r *http.Request) {
	c, ok := a.gctx(w, r)
	if !ok {
		return
	}
	name := clean(r.FormValue("name"), 30)
	if c.G != nil || name == "" {
		http.Redirect(w, r, c.url(""), http.StatusSeeOther)
		return
	}
	if choice := r.FormValue("choice"); strings.HasPrefix(choice, "join:") {
		sid, _ := strconv.ParseInt(strings.TrimPrefix(choice, "join:"), 10, 64)
		var members int
		a.db.QueryRow(`SELECT COUNT(*) FROM session_guests g JOIN table_sessions s ON s.id=g.session_id
			WHERE s.id=? AND s.table_id=? AND s.status IN ('open','bill_requested') AND g.status='joined'`, sid, c.Tbl.ID).Scan(&members)
		if members > 0 {
			// Joining an existing group needs an Allow tap from someone already in it.
			a.db.Exec(`INSERT INTO session_guests(session_id,display_name,device_token,status,last_seen) VALUES(?,?,?,'pending',?)`, sid, name, c.Dev, now())
			http.Redirect(w, r, c.url(""), http.StatusSeeOther)
			return
		}
	}
	a.newSession(c.R.ID, c.Tbl.ID, name, c.Dev, false)
	http.Redirect(w, r, c.url(""), http.StatusSeeOther)
}

func (a *App) newSession(rid, tableID int64, name, dev string, staff bool) int64 {
	sid, err := database.InsertID(a.db, `INSERT INTO table_sessions(restaurant_id,table_id,group_label,opened_at,last_activity) VALUES(?,?,?,?,?)`,
		rid, tableID, a.nextLabel(tableID), now(), now())
	if err != nil {
		return 0
	}
	a.db.Exec(`INSERT INTO session_guests(session_id,display_name,device_token,status,is_staff,last_seen) VALUES(?,?,?,'joined',?,?)`, sid, name, dev, staff, now())
	return sid
}

func (a *App) gWait(w http.ResponseWriter, r *http.Request) {
	c, ok := a.gctx(w, r)
	if !ok {
		return
	}
	if c.G != nil && c.G.Status == "joined" {
		w.Header().Set("HX-Redirect", c.url(""))
		return
	}
	m := c.data()
	m["Declined"] = c.G == nil
	a.render(w, "g_wait", m)
}

func (a *App) gLeave(w http.ResponseWriter, r *http.Request) {
	c, ok := a.gctx(w, r)
	if !ok {
		return
	}
	// Only a waiting guest or a guest whose bill is paid can leave; nobody walks away from an open bill here.
	if c.G != nil && (c.G.Status == "pending" || c.S.Status == "paid") {
		a.db.Exec(`UPDATE session_guests SET status='left' WHERE id=?`, c.G.ID)
	}
	http.Redirect(w, r, c.url(""), http.StatusSeeOther)
}

func (a *App) gLang(w http.ResponseWriter, r *http.Request) {
	l := "bn"
	if r.URL.Query().Get("l") == "en" {
		l = "en"
	}
	http.SetCookie(w, &http.Cookie{Name: "lang", Value: l, Path: "/", MaxAge: 365 * 86400, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/t/"+r.PathValue("tok"), http.StatusSeeOther)
}
