package server

import (
	"log"
	"net/http"
	"strconv"

	"bosho/internal/money"
)

func (a *App) ownerMenu(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "owner_menu", M{"Staff": s, "Page": "menu", "Cats": a.menu(s.R.ID)})
}

func (a *App) catCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	bn, en := clean(r.FormValue("name_bn"), 60), clean(r.FormValue("name_en"), 60)
	if bn != "" || en != "" {
		a.db.Exec(`INSERT INTO menu_categories(restaurant_id,name_bn,name_en,sort_order) VALUES(?,?,?,(SELECT COALESCE(MAX(sort_order),0)+1 FROM menu_categories WHERE restaurant_id=?))`,
			s.R.ID, bn, en, s.R.ID)
	}
	back(w, r, "/owner/menu")
}

func (a *App) catDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	// Only an empty category can be removed; items in old bills keep pointing at their category.
	a.db.Exec(`DELETE FROM menu_categories WHERE id=? AND restaurant_id=? AND NOT EXISTS (SELECT 1 FROM menu_items WHERE category_id=?)`, id, s.R.ID, id)
	back(w, r, "/owner/menu")
}

// savePhoto stores an uploaded JPEG, PNG or WebP in object storage and returns its name ("" when no usable file was sent).
func (a *App) savePhoto(r *http.Request) string {
	f, _, err := r.FormFile("photo")
	if err != nil {
		return ""
	}
	defer f.Close()
	name, err := a.photos.Save(r.Context(), f)
	if err != nil {
		log.Println("photo upload:", err)
		return ""
	}
	return name
}

func itemForm(r *http.Request) (cat int64, bn, en string, price int64, prep int) {
	cat, _ = strconv.ParseInt(r.FormValue("category_id"), 10, 64)
	bn, en = clean(r.FormValue("name_bn"), 80), clean(r.FormValue("name_en"), 80)
	price = money.Parse(r.FormValue("price"))
	prep, _ = strconv.Atoi(r.FormValue("prep_minutes"))
	if prep < 1 || prep > 240 {
		prep = 15
	}
	return
}

func (a *App) ownsCategory(rid, cat int64) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM menu_categories WHERE id=? AND restaurant_id=?`, cat, rid).Scan(&n)
	return n == 1
}

func (a *App) itemCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	r.ParseMultipartForm(6 << 20)
	cat, bn, en, price, prep := itemForm(r)
	if (bn != "" || en != "") && a.ownsCategory(s.R.ID, cat) {
		a.db.Exec(`INSERT INTO menu_items(restaurant_id,category_id,name_bn,name_en,price,photo,prep_minutes) VALUES(?,?,?,?,?,?,?)`,
			s.R.ID, cat, bn, en, price, a.savePhoto(r), prep)
	}
	back(w, r, "/owner/menu")
}

func (a *App) itemEdit(w http.ResponseWriter, r *http.Request, s *Staff) {
	it, ok := a.item(s.R.ID, pathID(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	a.render(w, "owner_item", M{"Staff": s, "Page": "menu", "It": it, "Cats": a.menu(s.R.ID)})
}

func (a *App) itemUpdate(w http.ResponseWriter, r *http.Request, s *Staff) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	r.ParseMultipartForm(6 << 20)
	id := pathID(r, "id")
	cat, bn, en, price, prep := itemForm(r)
	if (bn != "" || en != "") && a.ownsCategory(s.R.ID, cat) {
		a.db.Exec(`UPDATE menu_items SET category_id=?,name_bn=?,name_en=?,price=?,prep_minutes=? WHERE id=? AND restaurant_id=?`,
			cat, bn, en, price, prep, id, s.R.ID)
		if photo := a.savePhoto(r); photo != "" {
			a.db.Exec(`UPDATE menu_items SET photo=? WHERE id=? AND restaurant_id=?`, photo, id, s.R.ID)
		}
	}
	back(w, r, "/owner/menu")
}

func (a *App) itemDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	a.db.Exec(`UPDATE menu_items SET deleted=TRUE, is_available=FALSE WHERE id=? AND restaurant_id=?`, id, s.R.ID)
	a.db.Exec(`DELETE FROM cart_items WHERE menu_item_id=? AND EXISTS (SELECT 1 FROM menu_items WHERE id=? AND restaurant_id=?)`, id, id, s.R.ID)
	back(w, r, "/owner/menu")
}

func (a *App) optCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	name := clean(r.FormValue("name"), 40)
	if _, ok := a.item(s.R.ID, id); ok && name != "" {
		a.db.Exec(`INSERT INTO item_options(menu_item_id,name,extra_price) VALUES(?,?,?)`, id, name, money.Parse(r.FormValue("extra")))
	}
	back(w, r, "/owner/items/"+strconv.FormatInt(id, 10))
}

func (a *App) optDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	var itemID int64
	id := pathID(r, "id")
	a.db.QueryRow(`SELECT o.menu_item_id FROM item_options o JOIN menu_items m ON m.id=o.menu_item_id WHERE o.id=? AND m.restaurant_id=?`, id, s.R.ID).Scan(&itemID)
	if itemID == 0 {
		http.NotFound(w, r)
		return
	}
	a.db.Exec(`DELETE FROM item_options WHERE id=?`, id)
	back(w, r, "/owner/items/"+strconv.FormatInt(itemID, 10))
}
