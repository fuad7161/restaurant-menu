package server

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"bosho/internal/money"
)

func (a *App) ownerMenu(w http.ResponseWriter, r *http.Request, s *Staff) {
	// ?add=cat or ?add=dish opens that form; ?cat= preselects the dish's category.
	a.render(w, "owner_menu", M{"Staff": s, "Page": "menu", "Cats": a.menu(s.R.ID),
		"Add": r.URL.Query().Get("add"), "AddCat": queryID(r, "cat")})
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

// savePhoto stores an uploaded JPEG, PNG or WebP from the form field in object storage
// and returns its name ("" when no usable file was sent). Logos are kept as uploaded (they may be
// transparent and are drawn into QR codes); dish photos are resized (see storage.SaveDish).
func (a *App) savePhoto(r *http.Request, field string) string {
	f, _, err := r.FormFile(field)
	if err != nil {
		return ""
	}
	defer f.Close()
	save := a.photos.SaveDish
	if field == "logo" {
		save = a.photos.Save
	}
	name, err := save(r.Context(), f)
	if err != nil {
		log.Println("photo upload:", err)
		return ""
	}
	return name
}

type dishForm struct {
	cat                    int64
	bn, en, descBn, descEn string
	price                  int64
	prep                   int
}

func itemForm(r *http.Request) dishForm {
	var d dishForm
	d.cat, _ = strconv.ParseInt(r.FormValue("category_id"), 10, 64)
	d.bn, d.en = clean(r.FormValue("name_bn"), 80), clean(r.FormValue("name_en"), 80)
	d.descBn, d.descEn = clean(r.FormValue("desc_bn"), 300), clean(r.FormValue("desc_en"), 300)
	d.price = money.Parse(r.FormValue("price"))
	d.prep, _ = strconv.Atoi(r.FormValue("prep_minutes"))
	if d.prep < 1 || d.prep > 240 {
		d.prep = 15
	}
	return d
}

// tagsForm keeps only known tags, in their fixed order, and at most one spice level.
func tagsForm(r *http.Request) string {
	var out []string
	spice := r.FormValue("spice")
	for _, t := range dishTags {
		if strings.HasPrefix(t, "spicy") {
			if t == spice {
				out = append(out, t)
			}
		} else if r.FormValue("tag_"+t) == "1" {
			out = append(out, t)
		}
	}
	return strings.Join(out, ",")
}

func (a *App) ownsCategory(rid, cat int64) bool {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM menu_categories WHERE id=? AND restaurant_id=?`, cat, rid).Scan(&n)
	return n == 1
}

func (a *App) itemCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	r.ParseMultipartForm(6 << 20)
	d := itemForm(r)
	if (d.bn != "" || d.en != "") && a.ownsCategory(s.R.ID, d.cat) {
		a.db.Exec(`INSERT INTO menu_items(restaurant_id,category_id,name_bn,name_en,desc_bn,desc_en,price,photo,prep_minutes) VALUES(?,?,?,?,?,?,?,?,?)`,
			s.R.ID, d.cat, d.bn, d.en, d.descBn, d.descEn, d.price, a.savePhoto(r, "photo"), d.prep)
	}
	back(w, r, "/owner/menu")
}

func (a *App) itemEdit(w http.ResponseWriter, r *http.Request, s *Staff) {
	it, ok := a.item(s.R.ID, pathID(r, "id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	a.render(w, "owner_item", M{"Staff": s, "Page": "menu", "It": it, "Cats": a.menu(s.R.ID),
		"AllGroups": a.optionGroups(it.ID), "Saved": r.URL.Query().Get("saved") != ""})
}

func (a *App) itemUpdate(w http.ResponseWriter, r *http.Request, s *Staff) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	r.ParseMultipartForm(6 << 20)
	id := pathID(r, "id")
	d := itemForm(r)
	if (d.bn != "" || d.en != "") && a.ownsCategory(s.R.ID, d.cat) {
		a.db.Exec(`UPDATE menu_items SET category_id=?,name_bn=?,name_en=?,desc_bn=?,desc_en=?,price=?,prep_minutes=?,tags=?,featured=? WHERE id=? AND restaurant_id=?`,
			d.cat, d.bn, d.en, d.descBn, d.descEn, d.price, d.prep, tagsForm(r), r.FormValue("featured") == "1", id, s.R.ID)
		if photo := a.savePhoto(r, "photo"); photo != "" {
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
		// group 0 = a free extra; any other group must belong to this dish
		var group any
		if g, _ := strconv.ParseInt(r.FormValue("group_id"), 10, 64); g > 0 {
			var n int
			a.db.QueryRow(`SELECT COUNT(*) FROM option_groups WHERE id=? AND menu_item_id=?`, g, id).Scan(&n)
			if n == 1 {
				group = g
			}
		}
		a.db.Exec(`INSERT INTO item_options(menu_item_id,name,extra_price,group_id) VALUES(?,?,?,?)`, id, name, money.Parse(r.FormValue("extra")), group)
	}
	back(w, r, "/owner/items/"+strconv.FormatInt(id, 10)+"#options")
}

// groupRule turns the form's "rule" choice into minimum and maximum choices.
// one = exactly one (required), upto = up to N (optional), any = any number (optional).
func groupRule(r *http.Request) (minC, maxC int) {
	n, _ := strconv.Atoi(r.FormValue("max"))
	n = max(1, min(n, 20))
	switch r.FormValue("rule") {
	case "one":
		return 1, 1
	case "upto":
		return 0, n
	}
	return 0, 0
}

func (a *App) groupCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	name := clean(r.FormValue("name"), 40)
	if _, ok := a.item(s.R.ID, id); ok && name != "" {
		minC, maxC := groupRule(r)
		a.db.Exec(`INSERT INTO option_groups(menu_item_id,name,min_choices,max_choices) VALUES(?,?,?,?)`, id, name, minC, maxC)
	}
	back(w, r, "/owner/items/"+strconv.FormatInt(id, 10)+"#options")
}

// groupDelete removes a group; its options stay on the dish as free extras.
func (a *App) groupDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	var itemID int64
	id := pathID(r, "id")
	a.db.QueryRow(`SELECT g.menu_item_id FROM option_groups g JOIN menu_items m ON m.id=g.menu_item_id WHERE g.id=? AND m.restaurant_id=?`, id, s.R.ID).Scan(&itemID)
	if itemID == 0 {
		http.NotFound(w, r)
		return
	}
	tx, err := a.db.Begin()
	if err == nil {
		tx.Exec(`UPDATE item_options SET group_id=NULL WHERE group_id=?`, id)
		tx.Exec(`DELETE FROM option_groups WHERE id=?`, id)
		if err := tx.Commit(); err != nil {
			log.Println("delete option group:", err)
		}
	}
	back(w, r, "/owner/items/"+strconv.FormatInt(itemID, 10)+"#options")
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
	back(w, r, "/owner/items/"+strconv.FormatInt(itemID, 10)+"#options")
}
