package server

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"bosho/internal/database"
	"bosho/internal/i18n"
	"bosho/internal/money"
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
		WHERE c.session_id=? AND c.quantity>0 ORDER BY c.id`,
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
	a.renderPoll(w, r, "g_bar", m)
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
	if err := exec(a.db, `UPDATE session_guests SET status=? WHERE id=? AND session_id=? AND status='pending'`, status, pathID(r, "gid"), c.S.ID); err != nil {
		serverError(w, err)
		return
	}
	m := c.data()
	a.barData(c, m, r.URL.Query().Get("tab"))
	a.render(w, "g_bar", m)
}

// addData is what the small add control on a dish card (g_add) and the dish sheet (g_sheet) show.
// From is where the guest adds from: "card" (menu), "suggest" (cart suggestions) or "sheet".
// Page is the screen underneath: "menu" or "cart".
func addData(c *gctx, it Item, from, page string) M {
	if from != "sheet" && from != "suggest" {
		from = "card"
	}
	if page != "cart" {
		page = "menu"
	}
	return M{"Lang": c.Lang, "Tok": c.Tok, "It": it, "From": from, "Page": page, "Qty": 1, "Sel": map[int64]bool{}}
}

func (a *App) gAddControl(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	if it, ok := a.item(c.R.ID, pathID(r, "id")); ok {
		a.render(w, "g_add", addData(c, it, r.URL.Query().Get("from"), r.URL.Query().Get("page")))
	}
}

// gDish opens the dish sheet: big photo, description, options, quantity and note.
func (a *App) gDish(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	it, found := a.item(c.R.ID, pathID(r, "id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	m := addData(c, it, "sheet", r.URL.Query().Get("page"))
	m["Tags"] = itemTags(a.withPopular(c.R.ID, it))
	a.render(w, "g_sheet", m)
}

// withPopular marks a single dish Popular when it is among the best sellers (menu() does this for the whole menu).
func (a *App) withPopular(rid int64, it Item) Item {
	for _, id := range a.bestSellers(rid, 3) {
		if id == it.ID {
			it.Popular = true
		}
	}
	return it
}

// chosenOptions checks the options a guest picked against the dish's groups. Radio groups post as
// opt_<group id>, everything else as opt. It returns the picked options in menu order, or a message
// naming the first group whose rule is not met.
func chosenOptions(r *http.Request, it Item, lang string) ([]Opt, map[int64]bool, string) {
	picked := map[string]bool{}
	for _, v := range r.Form["opt"] {
		picked[v] = true
	}
	var sel []Opt
	ids := map[int64]bool{}
	msg := ""
	for _, g := range it.Groups {
		for _, v := range r.Form["opt_"+strconv.FormatInt(g.ID, 10)] {
			picked[v] = true
		}
		n := 0
		for _, o := range g.Options {
			if picked[strconv.FormatInt(o.ID, 10)] {
				sel = append(sel, o)
				ids[o.ID] = true
				n++
			}
		}
		if g.ID == 0 || msg != "" {
			continue
		}
		if n < g.Min {
			msg = fmt.Sprintf(i18n.T(lang, "err_choose"), g.Name)
		} else if g.Max > 0 && n > g.Max {
			msg = fmt.Sprintf(i18n.T(lang, "err_toomany"), g.Max, g.Name)
		}
	}
	return sel, ids, msg
}

func guestName(lang, bn, en string) string {
	if lang == "bn" && bn != "" || en == "" {
		return bn
	}
	return en
}

func (a *App) gCartAdd(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	r.ParseForm()
	itemID, _ := strconv.ParseInt(r.FormValue("item_id"), 10, 64)
	it, found := a.item(c.R.ID, itemID)
	if !found {
		http.NotFound(w, r)
		return
	}
	m := addData(c, it, r.FormValue("from"), r.FormValue("page"))
	from, page := m["From"].(string), m["Page"].(string)
	qty, _ := strconv.Atoi(r.FormValue("qty"))
	qty = max(1, min(qty, 20))
	note := clean(r.FormValue("note"), 120)
	sel, selIDs, optErr := chosenOptions(r, it, c.Lang)
	m["Qty"], m["Note"], m["Sel"] = qty, note, selIDs

	// fail shows a message where the guest tapped: in the sheet, or in place of the card's button.
	fail := func(msg string) {
		if from == "sheet" {
			m["Err"] = msg
			m["Tags"] = itemTags(a.withPopular(c.R.ID, it))
			a.render(w, "g_sheet", m)
			return
		}
		a.render(w, "g_msg", msg)
	}
	switch {
	case c.S.Status != "open":
		fail(i18n.T(c.Lang, "not_ordering"))
		return
	case !c.R.IsOpen:
		fail(i18n.T(c.Lang, "closed_now"))
		return
	case !it.Available:
		fail(i18n.T(c.Lang, "sold_out"))
		return
	case from != "sheet" && len(it.Groups) > 0:
		// A dish with options is added from its sheet only, so required choices are never skipped.
		http.Error(w, "choose options first", http.StatusBadRequest)
		return
	case optErr != "":
		fail(optErr)
		return
	}
	// The server looks up option names and prices; the phone only sends their ids.
	var names []string
	var extra int64
	for _, o := range sel {
		names = append(names, o.Name)
		extra += o.Extra
	}
	optText := strings.Join(names, ", ")
	name := guestName(c.Lang, it.NameBn, it.NameEn)

	if r.FormValue("confirm") != "1" {
		// Warn when someone else at the table already has this dish in the cart.
		var who string
		var have int
		err := a.db.QueryRow(`SELECT g.display_name,SUM(c.quantity) FROM cart_items c JOIN session_guests g ON g.id=c.guest_id
			WHERE c.session_id=? AND c.menu_item_id=? AND c.guest_id!=? AND c.quantity>0 GROUP BY g.id ORDER BY g.id LIMIT 1`, c.S.ID, it.ID, c.G.ID).Scan(&who, &have)
		if err == nil {
			m["Dup"] = fmt.Sprintf(i18n.T(c.Lang, "dup_warn"), who, have, name)
			if from == "sheet" {
				m["Tags"] = itemTags(a.withPopular(c.R.ID, it))
				a.render(w, "g_sheet", m)
			} else {
				a.render(w, "g_add", m)
			}
			return
		}
	}
	// Same dish, options and note from the same guest: one cart line with a bigger quantity.
	// A line removed a moment ago (quantity 0, waiting for Undo) comes back the same way.
	res, err := a.db.Exec(`UPDATE cart_items SET quantity=LEAST(GREATEST(quantity,0)+?,50) WHERE session_id=? AND guest_id=? AND menu_item_id=? AND options_text=? AND note=?`,
		qty, c.S.ID, c.G.ID, it.ID, optText, note)
	if err == nil {
		if n, _ := res.RowsAffected(); n == 0 {
			_, err = a.db.Exec(`INSERT INTO cart_items(session_id,guest_id,menu_item_id,quantity,options_text,options_extra,note) VALUES(?,?,?,?,?,?,?)`,
				c.S.ID, c.G.ID, it.ID, qty, optText, extra, note)
		}
	}
	if err != nil {
		log.Println("cart add:", err)
		fail(i18n.T(c.Lang, "error"))
		return
	}
	a.db.Exec(`UPDATE table_sessions SET last_activity=? WHERE id=?`, now(), c.S.ID)

	if from != "sheet" { // the sheet answers with nothing, which closes it
		m["Added"] = true
		a.render(w, "g_add", m)
	}
	a.renderToast(w, c.Lang, fmt.Sprintf(i18n.T(c.Lang, "added_toast"), qty, name), "", "")
	bar := c.data()
	a.barData(c, bar, page)
	bar["OOB"] = true
	a.render(w, "g_bar", bar)
	if page == "cart" {
		a.renderCartListOOB(w, c)
	}
}

// renderToast appends a short message to the page's toast area (out of band). With undoURL the
// toast gets an Undo button that posts there and swaps the answer into undoTarget.
func (a *App) renderToast(w http.ResponseWriter, lang, msg, undoURL, undoTarget string) {
	a.render(w, "toast", M{"Lang": lang, "Msg": msg, "UndoURL": undoURL, "UndoTarget": undoTarget})
}

func (a *App) renderCartListOOB(w http.ResponseWriter, c *gctx) {
	m := c.data()
	a.barData(c, m, "cart")
	a.cartTotals(c, m)
	m["OOB"] = true
	a.render(w, "g_cartlist", m)
}

// gName changes the name this phone orders under. Cart lines show the new name at once.
func (a *App) gName(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	if name := clean(r.FormValue("name"), 30); name != "" {
		a.db.Exec(`UPDATE session_guests SET display_name=? WHERE id=?`, name, c.G.ID)
	}
	http.Redirect(w, r, c.url("/cart"), http.StatusSeeOther)
}

func (a *App) gCart(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	m := c.data()
	a.barData(c, m, "cart")
	a.cartTotals(c, m)
	m["Err"] = r.URL.Query().Get("err")
	m["Suggest"] = a.suggestions(c.R.ID, m["Cart"].([]cartRow), 4)
	a.render(w, "guest_cart", m)
}

// cartTotals adds what the cart would cost with service charge and VAT, as the bill will show them.
func (a *App) cartTotals(c *gctx, m M) {
	sub := m["CartTotal"].(int64)
	svc, vat := money.Percent(sub, c.R.Service), money.Percent(sub, c.R.VAT)
	m["CartService"], m["CartVAT"], m["CartGrand"] = svc, vat, sub+svc+vat
	n := m["CartN"].(int)
	if n == 1 {
		m["SendQ"] = i18n.T(c.Lang, "confirm_send_one")
	} else {
		m["SendQ"] = fmt.Sprintf(i18n.T(c.Lang, "confirm_send"), n)
	}
}

// suggestions picks up to n available dishes to offer under the cart: first those most often
// ordered together with what is in the cart (last 60 days), then best sellers, then featured
// dishes. Dishes already in the cart are left out.
func (a *App) suggestions(rid int64, cart []cartRow, n int) []Item {
	if len(cart) == 0 {
		return nil
	}
	inCart := map[int64]bool{}
	args := []any{rid, now() - 60*86400}
	marks := make([]string, 0, len(cart))
	for _, row := range cart {
		if !inCart[row.ItemID] {
			inCart[row.ItemID] = true
			marks = append(marks, "?")
			args = append(args, row.ItemID)
		}
	}
	ids := database.List(a.db, `SELECT i2.menu_item_id FROM order_items i1
		JOIN order_items i2 ON i2.order_id=i1.order_id AND i2.menu_item_id!=i1.menu_item_id
		JOIN orders o ON o.id=i1.order_id
		WHERE o.restaurant_id=? AND o.placed_at>? AND o.status NOT IN ('rejected','cancelled') AND i1.menu_item_id IN (`+strings.Join(marks, ",")+`)
		GROUP BY i2.menu_item_id ORDER BY COUNT(*) DESC, i2.menu_item_id LIMIT 20`,
		func(r *sql.Rows, id *int64) error { return r.Scan(id) }, args...)
	ids = append(ids, a.bestSellers(rid, 10)...)
	byID := map[int64]Item{}
	var featured []int64
	for _, cat := range a.menu(rid) {
		for _, it := range cat.Items {
			byID[it.ID] = it
			if it.Featured {
				featured = append(featured, it.ID)
			}
		}
	}
	ids = append(ids, featured...)
	var out []Item
	for _, id := range ids {
		it, ok := byID[id]
		if !ok || !it.Available || inCart[id] {
			continue
		}
		inCart[id] = true // no repeats
		out = append(out, it)
		if len(out) == n {
			break
		}
	}
	return out
}

func (a *App) gCartList(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	m := c.data()
	a.barData(c, m, "cart")
	a.cartTotals(c, m)
	a.renderPoll(w, r, "g_cartlist", m)
}

func (a *App) gCartQty(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	id := pathID(r, "id")
	// Only the line removed last can be undone; older removed lines go for good.
	a.db.Exec(`DELETE FROM cart_items WHERE session_id=? AND quantity<=0 AND id!=?`, c.S.ID, id)
	if r.FormValue("d") == "-1" {
		var left int
		var bn, en string
		err := a.db.QueryRow(`UPDATE cart_items c SET quantity=GREATEST(c.quantity-1,0) FROM menu_items m
			WHERE c.id=? AND c.session_id=? AND c.quantity>0 AND m.id=c.menu_item_id RETURNING c.quantity,m.name_bn,m.name_en`, id, c.S.ID).Scan(&left, &bn, &en)
		a.gCartListFor(w, c)
		if err == nil && left == 0 {
			// The line stays hidden with quantity 0 until Undo brings it back or another removal clears it.
			a.renderToast(w, c.Lang, fmt.Sprintf(i18n.T(c.Lang, "removed_toast"), guestName(c.Lang, bn, en)),
				c.url("/cart/"+strconv.FormatInt(id, 10)+"/undo"), "#cartlist")
		}
		return
	}
	a.db.Exec(`UPDATE cart_items SET quantity=LEAST(quantity+1,50) WHERE id=? AND session_id=? AND quantity>0`, id, c.S.ID)
	a.gCartListFor(w, c)
}

// gCartUndo brings back a cart line that was just taken down to zero.
func (a *App) gCartUndo(w http.ResponseWriter, r *http.Request) {
	c, ok := a.joined(w, r)
	if !ok {
		return
	}
	if c.S.Status == "open" {
		a.db.Exec(`UPDATE cart_items SET quantity=1 WHERE id=? AND session_id=? AND quantity=0`, pathID(r, "id"), c.S.ID)
	}
	a.gCartListFor(w, c)
	bar := c.data()
	a.barData(c, bar, "cart")
	bar["OOB"] = true
	a.render(w, "g_bar", bar)
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
	a.cartTotals(c, m)
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
		fail(i18n.T(c.Lang, "error"))
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
			fail(fmt.Sprintf(i18n.T(c.Lang, "err_soldout"), guestName(c.Lang, row.NameBn, row.NameEn)))
			return
		}
	}
	var earlier int
	tx.QueryRow(`SELECT COUNT(*) FROM orders WHERE session_id=? AND status NOT IN ('rejected','cancelled')`, c.S.ID).Scan(&earlier)

	oid, err := database.InsertID(tx, `INSERT INTO orders(restaurant_id,session_id,status,placed_by,is_addon,placed_at) VALUES(?,?,'placed',?,?,?)`,
		c.R.ID, c.S.ID, c.G.Name, earlier > 0, now())
	if err != nil {
		fail(i18n.T(c.Lang, "error"))
		return
	}
	// Every write is checked: one failed line must not leave a half order behind.
	for _, row := range rows {
		if err := exec(tx, `INSERT INTO order_items(order_id,menu_item_id,name_bn,name_en,unit_price,quantity,options_text,note) VALUES(?,?,?,?,?,?,?,?)`,
			oid, row.ItemID, row.NameBn, row.NameEn, row.Unit, row.Qty, row.Options, row.Note); err != nil {
			fail(i18n.T(c.Lang, "error"))
			return
		}
	}
	// Orders wait for a staff Accept unless the restaurant turned that off, or a waiter placed the order.
	if !c.R.RequireAccept || c.G.IsStaff {
		if err := exec(tx, `UPDATE orders SET status='cooking', estimated_ready_at=? WHERE id=?`, now()+int64(estimateMinutes(tx, c.R.ID, oid))*60, oid); err != nil {
			fail(i18n.T(c.Lang, "error"))
			return
		}
	}
	if exec(tx, `DELETE FROM cart_items WHERE session_id=?`, c.S.ID) != nil ||
		exec(tx, `UPDATE table_sessions SET last_activity=? WHERE id=?`, now(), c.S.ID) != nil || tx.Commit() != nil {
		fail(i18n.T(c.Lang, "error"))
		return
	}
	http.Redirect(w, r, c.url("/orders"), http.StatusSeeOther)
}
