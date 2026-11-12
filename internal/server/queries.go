package server

import (
	"database/sql"

	"bosho/internal/database"
	"bosho/internal/money"
)

func (a *App) menu(rid int64) []Cat {
	cats := database.List(a.db, `SELECT id,name_bn,name_en FROM menu_categories WHERE restaurant_id=? ORDER BY sort_order,id`,
		func(r *sql.Rows, c *Cat) error { return r.Scan(&c.ID, &c.NameBn, &c.NameEn) }, rid)
	items := database.List(a.db, `SELECT id,category_id,name_bn,name_en,price,photo,prep_minutes,is_available FROM menu_items WHERE restaurant_id=? AND NOT deleted ORDER BY id`,
		func(r *sql.Rows, i *Item) error {
			return r.Scan(&i.ID, &i.CatID, &i.NameBn, &i.NameEn, &i.Price, &i.Photo, &i.Prep, &i.Available)
		}, rid)
	type optRow struct {
		Item int64
		Opt
	}
	opts := database.List(a.db, `SELECT o.menu_item_id,o.id,o.name,o.extra_price FROM item_options o JOIN menu_items m ON m.id=o.menu_item_id WHERE m.restaurant_id=? ORDER BY o.id`,
		func(r *sql.Rows, o *optRow) error { return r.Scan(&o.Item, &o.ID, &o.Name, &o.Extra) }, rid)
	byItem := map[int64][]Opt{}
	for _, o := range opts {
		byItem[o.Item] = append(byItem[o.Item], o.Opt)
	}
	for i := range cats {
		for _, it := range items {
			if it.CatID == cats[i].ID {
				it.Options = byItem[it.ID]
				cats[i].Items = append(cats[i].Items, it)
			}
		}
	}
	return cats
}

func (a *App) item(rid, id int64) (Item, bool) {
	var it Item
	err := a.db.QueryRow(`SELECT id,category_id,name_bn,name_en,price,photo,prep_minutes,is_available FROM menu_items WHERE id=? AND restaurant_id=? AND NOT deleted`, id, rid).
		Scan(&it.ID, &it.CatID, &it.NameBn, &it.NameEn, &it.Price, &it.Photo, &it.Prep, &it.Available)
	if err != nil {
		return it, false
	}
	it.Options = database.List(a.db, `SELECT id,name,extra_price FROM item_options WHERE menu_item_id=? ORDER BY id`,
		func(r *sql.Rows, o *Opt) error { return r.Scan(&o.ID, &o.Name, &o.Extra) }, id)
	return it, true
}

var orderStep = map[string]int{"placed": 1, "cooking": 2, "ready": 3, "served": 4}

func (a *App) loadOrders(where string, args ...any) []OView {
	os := database.List(a.db, `SELECT o.id,o.session_id,o.status,o.reject_reason,o.placed_by,o.is_addon,o.placed_at,o.estimated_ready_at,t.label,s.group_label
		FROM orders o JOIN table_sessions s ON s.id=o.session_id JOIN tables t ON t.id=s.table_id WHERE `+where+` ORDER BY o.id`,
		func(r *sql.Rows, o *OView) error {
			return r.Scan(&o.ID, &o.SessionID, &o.Status, &o.Reason, &o.PlacedBy, &o.Addon, &o.PlacedAt, &o.Est, &o.Table, &o.Group)
		}, args...)
	for i := range os {
		o := &os[i]
		o.N = i + 1
		o.Step = orderStep[o.Status]
		o.Items = database.List(a.db, `SELECT id,menu_item_id,name_bn,name_en,options_text,note,status,last_change,quantity,req_qty,unit_price FROM order_items WHERE order_id=? ORDER BY id`,
			func(r *sql.Rows, it *OItem) error {
				return r.Scan(&it.ID, &it.MenuItemID, &it.NameBn, &it.NameEn, &it.Options, &it.Note, &it.Status, &it.Last, &it.Qty, &it.ReqQty, &it.Unit)
			}, o.ID)
		for j := range o.Items {
			o.Items[j].Line = int64(o.Items[j].Qty) * o.Items[j].Unit
		}
	}
	return os
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
func closeEmptyOrder(q database.Querier, orderID int64) {
	q.Exec(`UPDATE orders SET status='cancelled' WHERE id=? AND status IN ('placed','cooking','ready')
		AND NOT EXISTS (SELECT 1 FROM order_items WHERE order_id=? AND status!='cancelled')`, orderID, orderID)
}
