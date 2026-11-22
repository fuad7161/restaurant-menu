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

func (a *App) liveData(c *gctx, flash string) M {
	m := c.data()
	a.barData(c, m, "orders")
	paid := c.S.Status == "paid"
	orders := a.loadOrders(`o.session_id=?`, c.S.ID)
	for i := range orders {
		o := &orders[i]
		if o.Status == "cooking" && o.Est > 0 {
			if rem := (o.Est - now() + 59) / 60; rem < 1 {
				o.ETA = i18n.T(c.Lang, "any_moment")
			} else {
				o.ETA = fmt.Sprintf("%s %d–%d %s", i18n.T(c.Lang, "ready_in"), rem, rem+5, i18n.T(c.Lang, "min"))
			}
			// Progress from placing the order to the estimated ready time; never shown as finished early.
			if span := o.Est - o.PlacedAt; span > 0 {
				o.Progress = int(max(3, min(95, (now()-o.PlacedAt)*100/span)))
			}
		}
		if o.Status == "ready" || o.Status == "served" {
			o.Progress = 100
		}
		// "Order #2 is sent" stays on screen for a minute after placing, for everyone in the group.
		if o.Status != "rejected" && o.Status != "cancelled" && now()-o.PlacedAt < 60 {
			m["Sent"] = fmt.Sprintf(i18n.T(c.Lang, "order_sent"), o.N)
		}
		for j := range o.Items {
			it := &o.Items[j]
			// Before the kitchen accepts, the guest changes the order directly; after that it is a request.
			it.Direct = o.Status == "placed"
			it.CanChange = !paid && it.Status == "active" && (o.Status == "placed" || o.Status == "cooking" || o.Status == "ready")
		}
	}
	m["Orders"], m["Bill"], m["Paid"], m["Flash"] = orders, a.bill(c.S.ID, c.R), paid, flash
	m["Calls"] = a.callButtons(c)
	if paid {
		var rating int
		err := a.db.QueryRow(`SELECT rating FROM reviews WHERE session_id=? AND menu_item_id IS NULL`, c.S.ID).Scan(&rating)
		m["Reviewed"] = err == nil
		// A happy guest is offered the restaurant's Google review page.
		m["AskGoogle"] = err == nil && rating >= 4 && c.R.ReviewURL != ""
	}
	return m
}

func (a *App) gOrders(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	a.render(w, "guest_orders", a.liveData(c, ""))
}

func (a *App) gOrdersLive(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	a.renderPoll(w, r, "g_live", a.liveData(c, ""))
}

func (a *App) gItemChange(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	var oid int64
	var ostatus, istatus string
	var qty int
	id := pathID(r, "id")
	err := a.db.QueryRow(`SELECT o.id,o.status,i.status,i.quantity FROM order_items i JOIN orders o ON o.id=i.order_id WHERE i.id=? AND o.session_id=?`, id, c.S.ID).
		Scan(&oid, &ostatus, &istatus, &qty)
	if err == nil && istatus == "active" && c.S.Status != "paid" {
		want := 0
		if r.FormValue("action") == "minus" && qty > 1 {
			want = qty - 1
		}
		var err error
		switch ostatus {
		case "placed": // kitchen has not accepted yet: change it right away
			if want == 0 {
				err = a.inTx(func(q database.Querier) error {
					if err := exec(q, `UPDATE order_items SET status='cancelled', cancelled_by=?, cancel_reason='guest cancelled before accept' WHERE id=? AND status='active'`, c.G.Name, id); err != nil {
						return err
					}
					return closeEmptyOrder(q, oid)
				})
			} else {
				err = exec(a.db, `UPDATE order_items SET quantity=? WHERE id=? AND status='active'`, want, id)
			}
		case "cooking", "ready": // staff must approve
			err = exec(a.db, `UPDATE order_items SET status='cancel_requested', req_qty=?, last_change='' WHERE id=? AND status='active'`, want, id)
		}
		if err != nil {
			serverError(w, err)
			return
		}
	}
	a.render(w, "g_live", a.liveData(c, ""))
}

// callCooldown is how long after staff answer a call the same button stays off, against repeated taps.
const callCooldown = 60

// callBtn is one of the call buttons on the orders screen, with what happened to its last call.
type callBtn struct {
	Reason, Icon, Note string
	Off, Done          bool
}

func (a *App) callButtons(c *gctx) []callBtn {
	type callRow struct {
		Reason, Status  string
		Created, DoneAt int64
	}
	rows := database.List(a.db, `SELECT reason,status,created_at,done_at FROM waiter_calls
		WHERE session_id=? AND (status='pending' OR done_at>?) ORDER BY id`,
		func(r *sql.Rows, x *callRow) error { return r.Scan(&x.Reason, &x.Status, &x.Created, &x.DoneAt) }, c.S.ID, now()-120)
	btns := []callBtn{{Reason: "waiter", Icon: "🔔"}, {Reason: "water", Icon: "💧"}, {Reason: "tissue", Icon: "🧻"}}
	for i := range btns {
		b := &btns[i]
		for _, x := range rows { // later rows win
			if x.Reason != b.Reason {
				continue
			}
			if x.Status == "pending" {
				t := time.Unix(x.Created, 0).In(a.loc)
				b.Note, b.Off, b.Done = fmt.Sprintf(i18n.T(c.Lang, "called_at"), clock12(t.Hour()*60+t.Minute())), true, false
			} else {
				b.Note, b.Off, b.Done = i18n.T(c.Lang, "call_done"), now()-x.DoneAt < callCooldown, true
			}
		}
	}
	return btns
}

func (a *App) gCall(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	reason := r.FormValue("reason")
	if reason != "water" && reason != "tissue" {
		reason = "waiter"
	}
	// One open call per reason, and a short pause after staff answered it.
	var busy int
	a.db.QueryRow(`SELECT COUNT(*) FROM waiter_calls WHERE session_id=? AND reason=? AND (status='pending' OR done_at>?)`,
		c.S.ID, reason, now()-callCooldown).Scan(&busy)
	if busy == 0 && c.S.Status != "paid" {
		if err := exec(a.db, `INSERT INTO waiter_calls(restaurant_id,session_id,reason,created_at) VALUES(?,?,?,?)`, c.R.ID, c.S.ID, reason, now()); err != nil {
			serverError(w, err)
			return
		}
	}
	a.render(w, "g_live", a.liveData(c, i18n.T(c.Lang, "waiter_coming")))
}

func (a *App) gBill(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	var err error
	if strings.HasSuffix(r.URL.Path, "/cancel") {
		if err = exec(a.db, `UPDATE table_sessions SET status='open' WHERE id=? AND status='bill_requested'`, c.S.ID); err == nil && c.S.Status == "bill_requested" {
			c.S.Status = "open"
		}
	} else if c.S.Status == "open" && len(a.bill(c.S.ID, c.R).Lines) > 0 {
		if err = exec(a.db, `UPDATE table_sessions SET status='bill_requested' WHERE id=? AND status='open'`, c.S.ID); err == nil {
			c.S.Status = "bill_requested"
		}
	}
	if err != nil {
		serverError(w, err)
		return
	}
	a.render(w, "g_live", a.liveData(c, ""))
}

func (a *App) gReview(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	var done int
	a.db.QueryRow(`SELECT COUNT(*) FROM reviews WHERE session_id=?`, c.S.ID).Scan(&done)
	rating, _ := strconv.Atoi(r.FormValue("rating"))
	if c.S.Status == "paid" && done == 0 && rating >= 1 && rating <= 5 {
		lines := a.bill(c.S.ID, c.R).Lines
		err := a.inTx(func(q database.Querier) error {
			if err := exec(q, `INSERT INTO reviews(restaurant_id,session_id,rating,comment,created_at) VALUES(?,?,?,?,?)`,
				c.R.ID, c.S.ID, rating, clean(r.FormValue("comment"), 500), now()); err != nil {
				return err
			}
			for _, l := range lines {
				if d, _ := strconv.Atoi(r.FormValue(fmt.Sprintf("dish_%d", l.MenuItemID))); d >= 1 && d <= 5 {
					if err := exec(q, `INSERT INTO reviews(restaurant_id,session_id,menu_item_id,rating,created_at) VALUES(?,?,?,?,?)`,
						c.R.ID, c.S.ID, l.MenuItemID, d, now()); err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			serverError(w, err)
			return
		}
	}
	http.Redirect(w, r, c.url("/orders"), http.StatusSeeOther)
}

// gReceipt is a printable receipt of a paid bill, which the guest can print or save as a PDF.
func (a *App) gReceipt(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	if c.S.Status != "paid" {
		http.Redirect(w, r, c.url("/orders"), http.StatusSeeOther)
		return
	}
	m := c.data()
	m["Bill"] = a.bill(c.S.ID, c.R)
	var method string
	var at int64
	a.db.QueryRow(`SELECT method,created_at FROM payments WHERE session_id=? ORDER BY id DESC LIMIT 1`, c.S.ID).Scan(&method, &at)
	if at == 0 {
		at = c.S.ClosedAt
	}
	m["Method"] = i18n.T(c.Lang, "m_"+method)
	m["PaidAt"] = time.Unix(at, 0).In(a.loc).Format("2 Jan 2006, 3:04 pm")
	m["Ref"] = fmt.Sprintf("%d-%d", c.R.ID, c.S.ID)
	a.render(w, "guest_receipt", m)
}
