package server

import (
	"log"

	"golang.org/x/crypto/bcrypt"

	"bosho/internal/database"
)

func (a *App) seedDemo() {
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM restaurants`).Scan(&n)
	if n > 0 {
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("demo1234"), bcrypt.DefaultCost)
	rid, err := database.InsertID(a.db, `INSERT INTO restaurants(name,slug,vat_percent,service_percent,created_at) VALUES('Demo Kitchen','demo',5,10,?)`, now())
	if err != nil {
		log.Println("demo data:", err)
		return
	}
	a.db.Exec(`INSERT INTO staff_users(restaurant_id,name,phone,password_hash,role) VALUES(?,'Demo Owner','01700000000',?,'owner')`, rid, string(hash))
	for _, l := range []string{"1", "2", "3", "4"} {
		a.db.Exec(`INSERT INTO tables(restaurant_id,label,qr_token) VALUES(?,?,?)`, rid, l, "demo-table-"+l)
	}
	type it struct {
		bn, en string
		price  int64
		prep   int
		opts   []Opt
	}
	cats := []struct {
		bn, en string
		items  []it
	}{
		{"বিরিয়ানি", "Biryani", []it{
			{"কাচ্চি বিরিয়ানি", "Kacchi Biryani", 32000, 15, []Opt{{Name: "Extra meat", Extra: 12000}, {Name: "Less spicy"}}},
			{"চিকেন বিরিয়ানি", "Chicken Biryani", 22000, 12, nil},
			{"তেহারি", "Tehari", 18000, 10, nil}}},
		{"কাবাব", "Kebab", []it{
			{"শিক কাবাব", "Seekh Kebab", 16000, 20, nil},
			{"চিকেন টিক্কা", "Chicken Tikka", 19000, 20, nil}}},
		{"পানীয়", "Drinks", []it{
			{"বোরহানি", "Borhani", 6000, 3, nil},
			{"লাচ্ছি", "Lassi", 8000, 5, []Opt{{Name: "Large", Extra: 3000}}},
			{"পানি", "Water", 2000, 1, nil}}},
		{"মিষ্টি", "Dessert", []it{
			{"ফিরনি", "Firni", 7000, 3, nil}}},
	}
	for i, c := range cats {
		cid, _ := database.InsertID(a.db, `INSERT INTO menu_categories(restaurant_id,name_bn,name_en,sort_order) VALUES(?,?,?,?)`, rid, c.bn, c.en, i)
		for _, m := range c.items {
			mid, _ := database.InsertID(a.db, `INSERT INTO menu_items(restaurant_id,category_id,name_bn,name_en,price,prep_minutes) VALUES(?,?,?,?,?,?)`, rid, cid, m.bn, m.en, m.price, m.prep)
			for _, o := range m.opts {
				a.db.Exec(`INSERT INTO item_options(menu_item_id,name,extra_price) VALUES(?,?,?)`, mid, o.Name, o.Extra)
			}
		}
	}
}
