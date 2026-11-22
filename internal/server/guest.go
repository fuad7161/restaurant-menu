package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bosho/internal/database"
	"bosho/internal/i18n"
)

const (
	sessionIdle  = 2 * 3600 // an open session with no orders expires after this many idle seconds
	receiptHours = 3 * 3600 // a paid guest keeps seeing the receipt this long
	orderGap     = 10       // minimum seconds between two orders from one group
)

func (a *App) guestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if a.goInside(w, r) { // logged-in staff skip the welcome page
			return
		}
		a.render(w, "home", nil)
	})
	mux.HandleFunc("GET /t/{tok}", a.gLanding)
	mux.HandleFunc("POST /t/{tok}/enter", a.gEnter)
	mux.HandleFunc("GET /t/{tok}/wait", a.poll(a.gWait))
	mux.HandleFunc("POST /t/{tok}/leave", a.gLeave)
	mux.HandleFunc("GET /t/{tok}/lang", a.gLang)
	mux.HandleFunc("GET /t/{tok}/bar", a.poll(a.gBar))
	mux.HandleFunc("POST /t/{tok}/join/{gid}/{act}", a.gJoinDecide)
	mux.HandleFunc("GET /t/{tok}/add/{id}", a.gAddControl)
	mux.HandleFunc("GET /t/{tok}/dish/{id}", a.gDish)
	mux.HandleFunc("POST /t/{tok}/cart/add", a.gCartAdd)
	mux.HandleFunc("GET /t/{tok}/cart", a.gCart)
	mux.HandleFunc("GET /t/{tok}/cart/list", a.poll(a.gCartList))
	mux.HandleFunc("POST /t/{tok}/cart/{id}/qty", a.gCartQty)
	mux.HandleFunc("POST /t/{tok}/cart/{id}/undo", a.gCartUndo)
	mux.HandleFunc("POST /t/{tok}/cart/{id}/note", a.gCartNote)
	mux.HandleFunc("POST /t/{tok}/name", a.gName)
	mux.HandleFunc("POST /t/{tok}/order", a.gPlaceOrder)
	mux.HandleFunc("GET /t/{tok}/orders", a.gOrders)
	mux.HandleFunc("GET /t/{tok}/orders/live", a.poll(a.gOrdersLive))
	mux.HandleFunc("POST /t/{tok}/items/{id}/change", a.gItemChange)
	mux.HandleFunc("POST /t/{tok}/call", a.gCall)
	mux.HandleFunc("POST /t/{tok}/bill", a.gBill)
	mux.HandleFunc("POST /t/{tok}/bill/cancel", a.gBill)
	mux.HandleFunc("POST /t/{tok}/review", a.gReview)
	mux.HandleFunc("GET /t/{tok}/receipt", a.gReceipt)
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

// infoData adds the opening hours, the "last orders at" notice near closing, and the line saying
// which charges come on top of the menu prices.
func (a *App) infoData(c *gctx, m M) {
	m["Hours"], m["LastCall"] = hoursInfo(c.R, c.Lang, time.Now().In(a.loc))
	switch {
	case c.R.VAT > 0 && c.R.Service > 0:
		m["PriceNote"] = fmt.Sprintf(i18n.T(c.Lang, "price_note_both"), pct(c.R.VAT), pct(c.R.Service))
	case c.R.VAT > 0:
		m["PriceNote"] = fmt.Sprintf(i18n.T(c.Lang, "price_note_vat"), pct(c.R.VAT))
	case c.R.Service > 0:
		m["PriceNote"] = fmt.Sprintf(i18n.T(c.Lang, "price_note_svc"), pct(c.R.Service))
	}
}

// parseHM reads "HH:MM" as minutes after midnight.
func parseHM(s string) (int, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// clock12 prints minutes after midnight as "10:30 pm".
func clock12(m int) string {
	h, suffix := (m/60)%24, "am"
	if h >= 12 {
		suffix = "pm"
	}
	if h = h % 12; h == 0 {
		h = 12
	}
	return fmt.Sprintf("%d:%02d %s", h, m%60, suffix)
}

// hoursInfo returns "Open 12:00 pm – 11:00 pm" when both times are set, and the last-orders notice
// during the 30 minutes before the last order time (or the closing time when none is set).
func hoursInfo(r Restaurant, lang string, at time.Time) (hours, lastCall string) {
	open, okOpen := parseHM(r.OpenTime)
	closeAt, okClose := parseHM(r.CloseTime)
	if okOpen && okClose {
		hours = fmt.Sprintf(i18n.T(lang, "hours"), clock12(open), clock12(closeAt))
	}
	cutoff, ok := parseHM(r.LastOrder)
	if !ok {
		cutoff, ok = closeAt, okClose
	}
	if ok {
		left := cutoff - (at.Hour()*60 + at.Minute())
		if left < -12*60 { // the cutoff is after midnight
			left += 24 * 60
		}
		if left >= 0 && left <= 30 {
			lastCall = fmt.Sprintf(i18n.T(lang, "last_order_at"), clock12(cutoff))
		}
	}
	return
}

func (c *gctx) url(path string) string { return "/t/" + c.Tok + path }

func deviceToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("bosho_dev"); err == nil && len(c.Value) >= 16 {
		return c.Value
	}
	tok := randToken(16)
	setCookie(w, r, &http.Cookie{Name: "bosho_dev", Value: tok, MaxAge: 365 * 86400, HttpOnly: true})
	return tok
}

func (a *App) gctx(w http.ResponseWriter, r *http.Request) (*gctx, bool) {
	c := &gctx{Tok: r.PathValue("tok"), Lang: i18n.Lang(r)}
	err := a.db.QueryRow(`SELECT t.id,t.label,r.id,r.name,r.vat_percent,r.service_percent,r.is_open,r.require_accept,r.logo,r.brand_color,r.tagline,r.address,r.phone,r.order_networks,`+restExtraCols+`
		FROM tables t JOIN restaurants r ON r.id=t.restaurant_id WHERE t.qr_token=? AND t.is_active`, c.Tok).
		Scan(append([]any{&c.Tbl.ID, &c.Tbl.Label, &c.R.ID, &c.R.Name, &c.R.VAT, &c.R.Service, &c.R.IsOpen, &c.R.RequireAccept,
			&c.R.Logo, &c.R.Color, &c.R.Tagline, &c.R.Address, &c.R.Phone, &c.R.Networks}, c.R.extraDest()...)...)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	if !a.guestNetworkOK(r, c.R.ID, c.R.Networks) {
		if r.Header.Get("HX-Request") != "" {
			w.Header().Set("HX-Redirect", c.url(""))
			return nil, false
		}
		m := c.data()
		if ip, ok := a.clientIP(r); ok {
			m["IP"] = ip.String()
		}
		w.WriteHeader(http.StatusForbidden)
		a.render(w, "guest_offsite", m)
		return nil, false
	}
	c.Dev = deviceToken(w, r)
	// Expire abandoned sessions that never ordered anything. This is the one write a GET makes,
	// so it tells the refreshing screens itself (POSTs are covered by bumpOnWrite).
	if res, err := a.db.Exec(`UPDATE table_sessions SET status='closed', closed_at=? WHERE table_id=? AND status='open' AND last_activity<?
		AND NOT EXISTS (SELECT 1 FROM orders o WHERE o.session_id=table_sessions.id AND o.status NOT IN ('rejected','cancelled'))`,
		now(), c.Tbl.ID, now()-sessionIdle); err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			a.changed()
		}
	}

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
	// "Take orders only during opening hours": outside them the menu shows as closed to guests.
	// A waiter ordering for a table is not held to it.
	if c.R.IsOpen && c.R.OrdersInHours && (c.G == nil || !c.G.IsStaff) && !withinHours(c.R, time.Now().In(a.loc)) {
		c.R.IsOpen = false
	}
	return c, true
}

// withinHours reports whether at is between the opening time and the last order time (or the
// closing time when no last order time is set). Hours past midnight work, for example 18:00 to
// 01:00. Without both times it is always true.
func withinHours(r Restaurant, at time.Time) bool {
	open, ok1 := parseHM(r.OpenTime)
	cutoff, ok2 := parseHM(r.LastOrder)
	if !ok2 {
		cutoff, ok2 = parseHM(r.CloseTime)
	}
	if !ok1 || !ok2 {
		return true
	}
	m := at.Hour()*60 + at.Minute()
	if open <= cutoff {
		return m >= open && m <= cutoff
	}
	return m >= open || m <= cutoff
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
		a.infoData(c, m)
		a.render(w, "guest_enter", m)
	case c.G.Status == "pending":
		a.render(w, "guest_wait", m)
	case c.S.Status == "paid":
		http.Redirect(w, r, c.url("/orders"), http.StatusSeeOther)
	default:
		a.barData(c, m, "menu")
		a.infoData(c, m)
		cats := a.menu(c.R.ID)
		var featured []Item
		for _, cat := range cats {
			for _, it := range cat.Items {
				if it.Featured && it.Available {
					featured = append(featured, it)
				}
			}
		}
		m["Cats"], m["Featured"] = cats, featured
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
			if err := exec(a.db, `INSERT INTO session_guests(session_id,display_name,device_token,status,last_seen) VALUES(?,?,?,'pending',?)`, sid, name, c.Dev, now()); err != nil {
				serverError(w, err)
				return
			}
			http.Redirect(w, r, c.url(""), http.StatusSeeOther)
			return
		}
	}
	if _, err := a.newSession(c.R.ID, c.Tbl.ID, name, c.Dev, false); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, c.url(""), http.StatusSeeOther)
}

// newSession starts a group at a table with its first member, both in one transaction, so a
// group can never exist without anyone in it.
func (a *App) newSession(rid, tableID int64, name, dev string, staff bool) (int64, error) {
	var sid int64
	err := a.inTx(func(q database.Querier) error {
		var err error
		sid, err = database.InsertID(q, `INSERT INTO table_sessions(restaurant_id,table_id,group_label,opened_at,last_activity) VALUES(?,?,?,?,?)`,
			rid, tableID, a.nextLabel(tableID), now(), now())
		if err != nil {
			return err
		}
		return exec(q, `INSERT INTO session_guests(session_id,display_name,device_token,status,is_staff,last_seen) VALUES(?,?,?,'joined',?,?)`, sid, name, dev, staff, now())
	})
	return sid, err
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
	a.renderPoll(w, r, "g_wait", m)
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
	setCookie(w, r, &http.Cookie{Name: "lang", Value: l, MaxAge: 365 * 86400})
	http.Redirect(w, r, "/t/"+r.PathValue("tok"), http.StatusSeeOther)
}
