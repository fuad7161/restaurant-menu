package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
		}
		for j := range o.Items {
			it := &o.Items[j]
			// Before the kitchen accepts, the guest changes the order directly; after that it is a request.
			it.Direct = o.Status == "placed"
			it.CanChange = !paid && it.Status == "active" && (o.Status == "placed" || o.Status == "cooking" || o.Status == "ready")
		}
	}
	m["Orders"], m["Bill"], m["Paid"], m["Flash"] = orders, a.bill(c.S.ID, c.R), paid, flash
	if paid {
		var n int
		a.db.QueryRow(`SELECT COUNT(*) FROM reviews WHERE session_id=?`, c.S.ID).Scan(&n)
		m["Reviewed"] = n > 0
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
	a.render(w, "g_live", a.liveData(c, ""))
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
		switch ostatus {
		case "placed": // kitchen has not accepted yet: change it right away
			if want == 0 {
				a.db.Exec(`UPDATE order_items SET status='cancelled', cancelled_by=?, cancel_reason='guest cancelled before accept' WHERE id=?`, c.G.Name, id)
				closeEmptyOrder(a.db, oid)
			} else {
				a.db.Exec(`UPDATE order_items SET quantity=? WHERE id=?`, want, id)
			}
		case "cooking", "ready": // staff must approve
			a.db.Exec(`UPDATE order_items SET status='cancel_requested', req_qty=?, last_change='' WHERE id=?`, want, id)
		}
	}
	a.render(w, "g_live", a.liveData(c, ""))
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
	var pending int
	a.db.QueryRow(`SELECT COUNT(*) FROM waiter_calls WHERE session_id=? AND reason=? AND status='pending'`, c.S.ID, reason).Scan(&pending)
	if pending == 0 && c.S.Status != "paid" {
		a.db.Exec(`INSERT INTO waiter_calls(restaurant_id,session_id,reason,created_at) VALUES(?,?,?,?)`, c.R.ID, c.S.ID, reason, now())
	}
	a.render(w, "g_live", a.liveData(c, i18n.T(c.Lang, "waiter_coming")))
}

func (a *App) gBill(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	if strings.HasSuffix(r.URL.Path, "/cancel") {
		a.db.Exec(`UPDATE table_sessions SET status='open' WHERE id=? AND status='bill_requested'`, c.S.ID)
		c.S.Status = "open"
	} else if c.S.Status == "open" && len(a.bill(c.S.ID, c.R).Lines) > 0 {
		a.db.Exec(`UPDATE table_sessions SET status='bill_requested' WHERE id=? AND status='open'`, c.S.ID)
		c.S.Status = "bill_requested"
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
		a.db.Exec(`INSERT INTO reviews(restaurant_id,session_id,rating,comment,created_at) VALUES(?,?,?,?,?)`,
			c.R.ID, c.S.ID, rating, clean(r.FormValue("comment"), 500), now())
		for _, l := range a.bill(c.S.ID, c.R).Lines {
			if d, _ := strconv.Atoi(r.FormValue(fmt.Sprintf("dish_%d", l.MenuItemID))); d >= 1 && d <= 5 {
				a.db.Exec(`INSERT INTO reviews(restaurant_id,session_id,menu_item_id,rating,created_at) VALUES(?,?,?,?,?)`,
					c.R.ID, c.S.ID, l.MenuItemID, d, now())
			}
		}
	}
	http.Redirect(w, r, c.url("/orders"), http.StatusSeeOther)
}
