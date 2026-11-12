package server

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"bosho/internal/database"
	"bosho/internal/i18n"
)

type cartRow struct {
	ID, ItemID                           int64
	NameBn, NameEn, Options, Note, Guest string
	Unit                                 int64
	Qty                                  int
	Available                            bool
}

func (a *App) cart(q database.Querier, sid int64) ([]cartRow, int, int64) {
	rows := database.List(q, `SELECT c.id,c.menu_item_id,m.name_bn,m.name_en,c.options_text,c.note,g.display_name,m.price+c.options_extra,c.quantity,(m.is_available AND NOT m.deleted)
		FROM cart_items c JOIN menu_items m ON m.id=c.menu_item_id JOIN session_guests g ON g.id=c.guest_id
		WHERE c.session_id=? ORDER BY c.id`,
		func(r *sql.Rows, c *cartRow) error {
			return r.Scan(&c.ID, &c.ItemID, &c.NameBn, &c.NameEn, &c.Options, &c.Note, &c.Guest, &c.Unit, &c.Qty, &c.Available)
		}, sid)
	n, total := 0, int64(0)
	for _, c := range rows {
		n += c.Qty
		total += int64(c.Qty) * c.Unit
	}
	return rows, n, total
}

// barData fills what the bottom navigation bar shows. tab is the screen the guest is on: menu, cart or orders.
func (a *App) barData(c *gctx, m M, tab string) {
	if tab != "cart" && tab != "orders" {
		tab = "menu"
	}
	m["Tab"] = tab
	rows, n, total := a.cart(a.db, c.S.ID)
	m["Cart"], m["CartN"], m["CartTotal"] = rows, n, total
	var active int
	a.db.QueryRow(`SELECT COUNT(*) FROM orders WHERE session_id=? AND status IN ('placed','cooking','ready')`, c.S.ID).Scan(&active)
	m["OrderN"] = active
	m["Joins"] = database.List(a.db, `SELECT id,display_name FROM session_guests WHERE session_id=? AND status='pending' ORDER BY id`,
		func(r *sql.Rows, g *Guest) error { return r.Scan(&g.ID, &g.Name) }, c.S.ID)
}

func (a *App) gBar(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	m := c.data()
	a.barData(c, m, r.URL.Query().Get("tab"))
	a.render(w, "g_bar", m)
}

func (a *App) gJoinDecide(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	status := "declined"
	if r.PathValue("act") == "allow" {
		status = "joined"
	}
	a.db.Exec(`UPDATE session_guests SET status=? WHERE id=? AND session_id=? AND status='pending'`, status, pathID(r, "gid"), c.S.ID)
	m := c.data()
	a.barData(c, m, r.URL.Query().Get("tab"))
	a.render(w, "g_bar", m)
}

func (a *App) gAddControl(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	if it, ok := a.item(c.R.ID, pathID(r, "id")); ok {
		a.render(w, "g_add", M{"Lang": c.Lang, "Tok": c.Tok, "It": it})
	}
}

func (a *App) gCartAdd(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	r.ParseForm()
	itemID, _ := strconv.ParseInt(r.FormValue("item_id"), 10, 64)
	it, found := a.item(c.R.ID, itemID)
	switch {
	case !found:
		http.NotFound(w, r)
		return
	case c.S.Status != "open":
		a.render(w, "g_msg", i18n.T(c.Lang, "not_ordering"))
		return
	case !c.R.IsOpen:
		a.render(w, "g_msg", i18n.T(c.Lang, "closed_now"))
		return
	case !it.Available:
		a.render(w, "g_msg", i18n.T(c.Lang, "sold_out"))
		return
	}
	// Options chosen for this item; the server looks up their names and prices.
	var names, sel []string
	var extra int64
	for _, v := range r.Form["opt"] {
		for _, o := range it.Options {
			if strconv.FormatInt(o.ID, 10) == v {
				names = append(names, o.Name)
				sel = append(sel, v)
				extra += o.Extra
			}
		}
	}
	optText := strings.Join(names, ", ")
	name := it.NameEn
	if c.Lang == "bn" && it.NameBn != "" || name == "" {
		name = it.NameBn
	}

	if r.FormValue("confirm") != "1" {
		// Warn when someone else at the table already has this dish in the cart.
		var who string
		var qty int
		err := a.db.QueryRow(`SELECT g.display_name,SUM(c.quantity) FROM cart_items c JOIN session_guests g ON g.id=c.guest_id
			WHERE c.session_id=? AND c.menu_item_id=? AND c.guest_id!=? GROUP BY g.id ORDER BY g.id LIMIT 1`, c.S.ID, it.ID, c.G.ID).Scan(&who, &qty)
		if err == nil {
			a.render(w, "g_add", M{"Lang": c.Lang, "Tok": c.Tok, "It": it, "Sel": sel,
				"Dup": fmt.Sprintf(i18n.T(c.Lang, "dup_warn"), who, qty, name)})
			return
		}
	}
	res, _ := a.db.Exec(`UPDATE cart_items SET quantity=quantity+1 WHERE session_id=? AND guest_id=? AND menu_item_id=? AND options_text=? AND note=''`,
		c.S.ID, c.G.ID, it.ID, optText)
	if n, _ := res.RowsAffected(); n == 0 {
		a.db.Exec(`INSERT INTO cart_items(session_id,guest_id,menu_item_id,quantity,options_text,options_extra) VALUES(?,?,?,1,?,?)`,
			c.S.ID, c.G.ID, it.ID, optText, extra)
	}
	a.db.Exec(`UPDATE table_sessions SET last_activity=? WHERE id=?`, now(), c.S.ID)

	a.render(w, "g_add", M{"Lang": c.Lang, "Tok": c.Tok, "It": it, "Added": true})
	m := c.data()
	a.barData(c, m, "menu")
	m["OOB"] = true
	a.render(w, "g_bar", m)
}

func (a *App) gCart(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	m := c.data()
	a.barData(c, m, "cart")
	m["Err"] = r.URL.Query().Get("err")
	a.render(w, "guest_cart", m)
}

func (a *App) gCartList(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	m := c.data()
	a.barData(c, m, "cart")
	a.render(w, "g_cartlist", m)
}

func (a *App) gCartQty(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	d := 1
	if r.FormValue("d") == "-1" {
		d = -1
	}
	a.db.Exec(`UPDATE cart_items SET quantity=LEAST(quantity+?,50) WHERE id=? AND session_id=?`, d, pathID(r, "id"), c.S.ID)
	a.db.Exec(`DELETE FROM cart_items WHERE id=? AND session_id=? AND quantity<=0`, pathID(r, "id"), c.S.ID)
	a.gCartListFor(w, c)
}

func (a *App) gCartNote(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	a.db.Exec(`UPDATE cart_items SET note=? WHERE id=? AND session_id=?`, clean(r.FormValue("note"), 120), pathID(r, "id"), c.S.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) gCartListFor(w http.ResponseWriter, c *gctx) {
	m := c.data()
	a.barData(c, m, "cart")
	a.render(w, "g_cartlist", m)
}

func (a *App) gPlaceOrder(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	fail := func(msg string) {
		http.Redirect(w, r, c.url("/cart?err=")+url.QueryEscape(msg), http.StatusSeeOther)
	}
	if c.S.Status != "open" {
		fail(i18n.T(c.Lang, "not_ordering"))
		return
	}
	if !c.R.IsOpen {
		fail(i18n.T(c.Lang, "closed_now"))
		return
	}
	// One transaction turns the shared cart into an order, so two taps cannot create two orders.
	tx, err := a.db.Begin()
	if err != nil {
		fail("error")
		return
	}
	defer tx.Rollback()

	// Lock the group's row so two phones tapping Place order at once are handled one after the other.
	var locked int64
	if tx.QueryRow(`SELECT id FROM table_sessions WHERE id=? AND status='open' FOR UPDATE`, c.S.ID).Scan(&locked) != nil {
		fail(i18n.T(c.Lang, "not_ordering"))
		return
	}
	var last int64
	tx.QueryRow(`SELECT COALESCE(MAX(placed_at),0) FROM orders WHERE session_id=?`, c.S.ID).Scan(&last)
	rows, _, _ := a.cart(tx, c.S.ID)
	if len(rows) == 0 {
		http.Redirect(w, r, c.url("/orders"), http.StatusSeeOther)
		return
	}
	if now()-last < orderGap {
		fail(i18n.T(c.Lang, "err_wait"))
		return
	}
	for _, row := range rows {
		if !row.Available {
			name := row.NameEn
			if c.Lang == "bn" && row.NameBn != "" || name == "" {
				name = row.NameBn
			}
			fail(fmt.Sprintf(i18n.T(c.Lang, "err_soldout"), name))
			return
		}
	}
	var earlier int
	tx.QueryRow(`SELECT COUNT(*) FROM orders WHERE session_id=? AND status NOT IN ('rejected','cancelled')`, c.S.ID).Scan(&earlier)

	oid, err := database.InsertID(tx, `INSERT INTO orders(restaurant_id,session_id,status,placed_by,is_addon,placed_at) VALUES(?,?,'placed',?,?,?)`,
		c.R.ID, c.S.ID, c.G.Name, earlier > 0, now())
	if err != nil {
		fail("error")
		return
	}
	for _, row := range rows {
		tx.Exec(`INSERT INTO order_items(order_id,menu_item_id,name_bn,name_en,unit_price,quantity,options_text,note) VALUES(?,?,?,?,?,?,?,?)`,
			oid, row.ItemID, row.NameBn, row.NameEn, row.Unit, row.Qty, row.Options, row.Note)
	}
	// Orders wait for a staff Accept unless the restaurant turned that off, or a waiter placed the order.
	if !c.R.RequireAccept || c.G.IsStaff {
		tx.Exec(`UPDATE orders SET status='cooking', estimated_ready_at=? WHERE id=?`, now()+int64(estimateMinutes(tx, c.R.ID, oid))*60, oid)
	}
	tx.Exec(`DELETE FROM cart_items WHERE session_id=?`, c.S.ID)
	tx.Exec(`UPDATE table_sessions SET last_activity=? WHERE id=?`, now(), c.S.ID)
	if err := tx.Commit(); err != nil {
		fail("error")
		return
	}
	http.Redirect(w, r, c.url("/orders"), http.StatusSeeOther)
}
