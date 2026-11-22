package server

import (
	"database/sql"

	"bosho/internal/database"
	"bosho/internal/money"
)

const itemCols = `id,category_id,name_bn,name_en,desc_bn,desc_en,price,photo,prep_minutes,is_available,featured,tags`

func scanItem(r interface{ Scan(...any) error }, i *Item) error {
	return r.Scan(&i.ID, &i.CatID, &i.NameBn, &i.NameEn, &i.DescBn, &i.DescEn, &i.Price, &i.Photo, &i.Prep, &i.Available, &i.Featured, &i.Tags)
}

func (a *App) menu(rid int64) []Cat {
	cats := database.List(a.db, `SELECT id,name_bn,name_en FROM menu_categories WHERE restaurant_id=? ORDER BY sort_order,id`,
		func(r *sql.Rows, c *Cat) error { return r.Scan(&c.ID, &c.NameBn, &c.NameEn) }, rid)
	// Featured dishes come first in their category.
	items := database.List(a.db, `SELECT `+itemCols+` FROM menu_items WHERE restaurant_id=? AND NOT deleted ORDER BY featured DESC, id`,
		func(r *sql.Rows, i *Item) error { return scanItem(r, i) }, rid)
	type optRow struct {
		Item int64
		Opt
	}
	opts := database.List(a.db, `SELECT o.menu_item_id,o.id,o.name,o.extra_price,COALESCE(o.group_id,0) FROM item_options o JOIN menu_items m ON m.id=o.menu_item_id WHERE m.restaurant_id=? ORDER BY o.id`,
		func(r *sql.Rows, o *optRow) error { return r.Scan(&o.Item, &o.ID, &o.Name, &o.Extra, &o.GroupID) }, rid)
	type groupRow struct {
		Item int64
		OptGroup
	}
	groups := database.List(a.db, `SELECT g.menu_item_id,g.id,g.name,g.min_choices,g.max_choices FROM option_groups g JOIN menu_items m ON m.id=g.menu_item_id WHERE m.restaurant_id=? ORDER BY g.id`,
		func(r *sql.Rows, g *groupRow) error { return r.Scan(&g.Item, &g.ID, &g.Name, &g.Min, &g.Max) }, rid)
	popular := map[int64]bool{}
	for _, id := range a.bestSellers(rid, 3) {
		popular[id] = true
	}
	optsBy, groupsBy := map[int64][]Opt{}, map[int64][]OptGroup{}
	for _, o := range opts {
		optsBy[o.Item] = append(optsBy[o.Item], o.Opt)
	}
	for _, g := range groups {
		groupsBy[g.Item] = append(groupsBy[g.Item], g.OptGroup)
	}
	for i := range cats {
		for _, it := range items {
			if it.CatID == cats[i].ID {
				it.Options = optsBy[it.ID]
				it.Groups = arrangeOptions(groupsBy[it.ID], it.Options)
				it.Popular = popular[it.ID]
				cats[i].Items = append(cats[i].Items, it)
			}
		}
	}
	return cats
}

// arrangeOptions puts each option into its group, keeps only groups that have options, and adds
// the options without a group last as free extras (group ID 0).
func arrangeOptions(groups []OptGroup, opts []Opt) []OptGroup {
	var out []OptGroup
	known := map[int64]bool{}
	for _, g := range groups {
		known[g.ID] = true
		for _, o := range opts {
			if o.GroupID == g.ID {
				g.Options = append(g.Options, o)
			}
		}
		if len(g.Options) > 0 {
			out = append(out, g)
		}
	}
	var free []Opt
	for _, o := range opts {
		if o.GroupID == 0 || !known[o.GroupID] {
			free = append(free, o)
		}
	}
	if len(free) > 0 {
		out = append(out, OptGroup{Options: free})
	}
	return out
}

// bestSellers returns up to n dish ids with the most portions sold in the last 30 days (3 or more each).
func (a *App) bestSellers(rid int64, n int) []int64 {
	return database.List(a.db, `SELECT i.menu_item_id FROM order_items i JOIN orders o ON o.id=i.order_id
		WHERE o.restaurant_id=? AND o.placed_at>? AND o.status NOT IN ('rejected','cancelled') AND i.status!='cancelled'
		GROUP BY i.menu_item_id HAVING SUM(i.quantity)>=3 ORDER BY SUM(i.quantity) DESC, i.menu_item_id LIMIT ?`,
		func(r *sql.Rows, id *int64) error { return r.Scan(id) }, rid, now()-30*86400, n)
}

func (a *App) item(rid, id int64) (Item, bool) {
	var it Item
	if scanItem(a.db.QueryRow(`SELECT `+itemCols+` FROM menu_items WHERE id=? AND restaurant_id=? AND NOT deleted`, id, rid), &it) != nil {
		return it, false
	}
	it.Options = database.List(a.db, `SELECT id,name,extra_price,COALESCE(group_id,0) FROM item_options WHERE menu_item_id=? ORDER BY id`,
		func(r *sql.Rows, o *Opt) error { return r.Scan(&o.ID, &o.Name, &o.Extra, &o.GroupID) }, id)
	it.Groups = arrangeOptions(a.optionGroups(id), it.Options)
	return it, true
}

// optionGroups lists a dish's option groups, including empty ones (for the owner's edit page).
func (a *App) optionGroups(itemID int64) []OptGroup {
	return database.List(a.db, `SELECT id,name,min_choices,max_choices FROM option_groups WHERE menu_item_id=? ORDER BY id`,
		func(r *sql.Rows, g *OptGroup) error { return r.Scan(&g.ID, &g.Name, &g.Min, &g.Max) }, itemID)
}

var orderStep = map[string]int{"placed": 1, "cooking": 2, "ready": 3, "served": 4}

func (a *App) loadOrders(where string, args ...any) []OView {
	os := database.List(a.db, `SELECT o.id,o.session_id,o.status,o.reject_reason,o.placed_by,o.is_addon,o.placed_at,o.estimated_ready_at,o.ready_at,o.served_at,t.label,s.group_label
		FROM orders o JOIN table_sessions s ON s.id=o.session_id JOIN tables t ON t.id=s.table_id WHERE `+where+` ORDER BY o.id`,
		func(r *sql.Rows, o *OView) error {
			return r.Scan(&o.ID, &o.SessionID, &o.Status, &o.Reason, &o.PlacedBy, &o.Addon, &o.PlacedAt, &o.Est, &o.ReadyAt, &o.ServedAt, &o.Table, &o.Group)
		}, args...)
	if len(os) == 0 {
		return os
	}
	// All lines of all these orders in one query (the kitchen board can hold dozens of orders).
	ids := make([]int64, len(os))
	byOrder := map[int64]*OView{}
	for i := range os {
		os[i].N = i + 1
		os[i].Step = orderStep[os[i].Status]
		ids[i] = os[i].ID
		byOrder[os[i].ID] = &os[i]
	}
	type line struct {
		Order int64
		OItem
	}
	lines := database.List(a.db, `SELECT order_id,id,menu_item_id,name_bn,name_en,options_text,note,status,last_change,quantity,req_qty,unit_price
		FROM order_items WHERE order_id = ANY(?) ORDER BY id`,
		func(r *sql.Rows, l *line) error {
			return r.Scan(&l.Order, &l.ID, &l.MenuItemID, &l.NameBn, &l.NameEn, &l.Options, &l.Note, &l.Status, &l.Last, &l.Qty, &l.ReqQty, &l.Unit)
		}, ids)
	for _, l := range lines {
		l.Line = int64(l.Qty) * l.Unit
		if o := byOrder[l.Order]; o != nil {
			o.Items = append(o.Items, l.OItem)
		}
	}
	return os
}

// estimateAll is estimateMinutes for many waiting orders at once, with two queries in total.
func (a *App) estimateAll(rid int64, orderIDs []int64) map[int64]int {
	out := map[int64]int{}
	if len(orderIDs) == 0 {
		return out
	}
	var queue int
	a.db.QueryRow(`SELECT COUNT(*) FROM orders WHERE restaurant_id=? AND status='cooking'`, rid).Scan(&queue)
	type prep struct {
		Order int64
		Mins  int
	}
	for _, p := range database.List(a.db, `SELECT i.order_id,COALESCE(MAX(m.prep_minutes),15) FROM order_items i JOIN menu_items m ON m.id=i.menu_item_id
		WHERE i.order_id = ANY(?) AND i.status!='cancelled' GROUP BY i.order_id`,
		func(r *sql.Rows, p *prep) error { return r.Scan(&p.Order, &p.Mins) }, orderIDs) {
		out[p.Order] = p.Mins + min(queue*3, 30)
	}
	return out
}

func (a *App) bill(sid int64, r Restaurant) Bill {
	var b Bill
	b.Lines = database.List(a.db, `SELECT i.menu_item_id,i.name_bn,i.name_en,SUM(i.quantity),SUM(i.quantity*i.unit_price)::BIGINT
		FROM order_items i JOIN orders o ON o.id=i.order_id
		WHERE o.session_id=? AND o.status NOT IN ('rejected','cancelled') AND i.status!='cancelled'
		GROUP BY i.menu_item_id,i.name_bn,i.name_en,i.unit_price ORDER BY MIN(i.id)`,
		func(rw *sql.Rows, l *BillLine) error {
			return rw.Scan(&l.MenuItemID, &l.NameBn, &l.NameEn, &l.Qty, &l.Amount)
		}, sid)
	for _, l := range b.Lines {
		b.Subtotal += l.Amount
	}
	b.Service = money.Percent(b.Subtotal, r.Service)
	b.VAT = money.Percent(b.Subtotal, r.VAT)
	b.Total = b.Subtotal + b.Service + b.VAT
	return b
}

// estimateMinutes: longest prep time in the order plus 3 minutes per order already cooking.
func estimateMinutes(q database.Querier, rid, orderID int64) int {
	prep, queue := 15, 0
	q.QueryRow(`SELECT COALESCE(MAX(m.prep_minutes),15) FROM order_items i JOIN menu_items m ON m.id=i.menu_item_id WHERE i.order_id=? AND i.status!='cancelled'`, orderID).Scan(&prep)
	q.QueryRow(`SELECT COUNT(*) FROM orders WHERE restaurant_id=? AND status='cooking' AND id!=?`, rid, orderID).Scan(&queue)
	return prep + min(queue*3, 30)
}

// closeEmptyOrder cancels an unserved order once every item in it is cancelled.
func closeEmptyOrder(q database.Querier, orderID int64) error {
	return exec(q, `UPDATE orders SET status='cancelled' WHERE id=? AND status IN ('placed','cooking','ready')
		AND NOT EXISTS (SELECT 1 FROM order_items WHERE order_id=? AND status!='cancelled')`, orderID, orderID)
}
