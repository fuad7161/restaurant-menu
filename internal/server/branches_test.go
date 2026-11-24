package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bosho/internal/money"
)

// Branches (docs/plans/multi-branch.md, section 7). The demo business starts with one branch,
// "Demo Kitchen", owned by 01700000000.

func demoOwner(t *testing.T, srv *httptest.Server) *tclient {
	t.Helper()
	c := newTClient(t, srv)
	c.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	return c
}

// addGulshan adds a second demo branch, copying everything from the first, and returns its id.
func addGulshan(t *testing.T, a *App, owner *tclient) int64 {
	t.Helper()
	owner.do("POST", "/owner/branches", url.Values{"branch_name": {"Gulshan"}, "code": {"gul"}, "from": {fmt.Sprint(demoID(a))},
		"copy_menu": {"1"}, "copy_settings": {"1"}, "copy_brand": {"1"}})
	var id int64
	if err := a.db.QueryRow(`SELECT id FROM restaurants WHERE branch_name='Gulshan'`).Scan(&id); err != nil {
		t.Fatalf("branch not added: %v", err)
	}
	return id
}

func demoID(a *App) int64 {
	var id int64
	a.db.QueryRow(`SELECT id FROM restaurants WHERE slug='demo'`).Scan(&id)
	return id
}

func switchTo(c *tclient, id any) {
	c.do("POST", "/owner/branch", url.Values{"branch": {fmt.Sprint(id)}})
}

func TestBranchCopiesTheMenu(t *testing.T) {
	a, srv := testApp(t)
	demo := demoID(a)
	// Give Kacchi an option group, so the copy has to put options into the new group.
	var kacchi int64
	a.db.QueryRow(`SELECT id FROM menu_items WHERE name_en='Kacchi Biryani'`).Scan(&kacchi)
	var gid int64
	a.db.QueryRow(`INSERT INTO option_groups(menu_item_id,name,min_choices,max_choices) VALUES(?,'Size',1,1) RETURNING id`, kacchi).Scan(&gid)
	a.db.Exec(`INSERT INTO item_options(menu_item_id,name,extra_price,group_id) VALUES(?,'Half',0,?),(?,'Full',15000,?)`, kacchi, gid, kacchi, gid)

	owner := demoOwner(t, srv)
	gul := addGulshan(t, a, owner)

	count := func(q string, rid int64) int {
		var n int
		a.db.QueryRow(q, rid).Scan(&n)
		return n
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM menu_categories WHERE restaurant_id=?`,
		`SELECT COUNT(*) FROM menu_items WHERE restaurant_id=? AND NOT deleted`,
		`SELECT COUNT(*) FROM option_groups g JOIN menu_items m ON m.id=g.menu_item_id WHERE m.restaurant_id=?`,
		`SELECT COUNT(*) FROM item_options o JOIN menu_items m ON m.id=o.menu_item_id WHERE m.restaurant_id=?`,
	} {
		if d, g := count(q, demo), count(q, gul); d != g || d == 0 {
			t.Errorf("%s: demo %d, Gulshan %d", q, d, g)
		}
	}
	// Options stay in their own dish's group, never in the original branch's group.
	var wrong int
	a.db.QueryRow(`SELECT COUNT(*) FROM item_options o JOIN menu_items m ON m.id=o.menu_item_id JOIN option_groups g ON g.id=o.group_id
		WHERE m.restaurant_id=? AND g.menu_item_id!=o.menu_item_id`, gul).Scan(&wrong)
	var rule string
	a.db.QueryRow(`SELECT g.name||g.min_choices||g.max_choices||COUNT(o.id) FROM option_groups g JOIN menu_items m ON m.id=g.menu_item_id
		JOIN item_options o ON o.group_id=g.id WHERE m.restaurant_id=? GROUP BY g.id`, gul).Scan(&rule)
	if wrong != 0 || rule != "Size112" {
		t.Errorf("copied group: %d options in a wrong group, rule %q", wrong, rule)
	}
	// Settings came along; tables and staff did not.
	var vat, service float64
	a.db.QueryRow(`SELECT vat_percent,service_percent FROM restaurants WHERE id=?`, gul).Scan(&vat, &service)
	if vat != 5 || service != 10 || count(`SELECT COUNT(*) FROM tables WHERE restaurant_id=?`, gul) != 0 {
		t.Errorf("Gulshan: VAT %v, service %v, %d tables", vat, service, count(`SELECT COUNT(*) FROM tables WHERE restaurant_id=?`, gul))
	}
	// After the copy, each branch's menu is its own.
	switchTo(owner, gul)
	var gulKacchi int64
	a.db.QueryRow(`SELECT id FROM menu_items WHERE restaurant_id=? AND name_en='Kacchi Biryani'`, gul).Scan(&gulKacchi)
	a.db.Exec(`UPDATE menu_items SET price=1 WHERE id=?`, gulKacchi)
	var price int64
	a.db.QueryRow(`SELECT price FROM menu_items WHERE id=?`, kacchi).Scan(&price)
	if price != 32000 {
		t.Errorf("changing Gulshan's price changed Demo Kitchen's to %d", price)
	}
}

func TestOwnerSwitchesBranches(t *testing.T) {
	a, srv := testApp(t)
	owner := demoOwner(t, srv)
	gul := addGulshan(t, a, owner)

	// A new login with two branches starts in "All branches": one-branch pages ask for a branch.
	owner = demoOwner(t, srv)
	if !strings.Contains(owner.get("/owner/tables"), "Pick a branch") {
		t.Fatal("All branches mode opened the tables page of some branch")
	}
	if !strings.Contains(owner.get("/owner/branches"), "Gulshan") {
		t.Error("Branches page does not list Gulshan")
	}
	switchTo(owner, demoID(a))
	if !strings.Contains(owner.get("/owner/tables"), "demo-table-1") {
		t.Error("Demo Kitchen's tables page does not show its tables")
	}
	switchTo(owner, gul)
	if strings.Contains(owner.get("/owner/tables"), "demo-table-1") {
		t.Error("Gulshan's tables page shows Demo Kitchen's tables")
	}
	// The counter of Gulshan does not show Demo Kitchen's bills.
	g := newTClient(t, srv)
	g.get("/t/demo-table-2")
	g.do("POST", "/t/demo-table-2/enter", url.Values{"name": {"Rafi"}, "choice": {"new"}})
	var sid int64
	a.db.QueryRow(`SELECT id FROM table_sessions ORDER BY id DESC LIMIT 1`).Scan(&sid)
	if code, _ := owner.do("GET", fmt.Sprintf("/staff/sessions/%d", sid), nil); code != http.StatusNotFound {
		t.Errorf("Gulshan opened Demo Kitchen's bill: %d", code)
	}
}

func TestBranchesStayInsideTheirBusiness(t *testing.T) {
	a, srv := testApp(t)
	other := newTClient(t, srv)
	other.do("POST", "/signup", url.Values{"restaurant": {"Star Kabab"}, "name": {"Karim"}, "phone": {"01920000000"},
		"email": {"karim@example.com"}, "password": {"secretpass"}})
	if code, _ := other.do("POST", "/owner/branch", url.Values{"branch": {fmt.Sprint(demoID(a))}}); code != http.StatusNotFound {
		t.Errorf("another business's owner switched to the demo branch: %d", code)
	}
	if strings.Contains(other.get("/owner/tables"), "demo-table-1") {
		t.Error("another business's owner sees the demo tables")
	}
	code, _ := other.do("POST", fmt.Sprintf("/owner/branches/%d/status", demoID(a)), url.Values{"status": {"closed"}})
	var status string
	a.db.QueryRow(`SELECT status FROM restaurants WHERE id=?`, demoID(a)).Scan(&status)
	if code != http.StatusNotFound || status != "active" {
		t.Errorf("another business's owner closed the demo branch: %d, %s", code, status)
	}
}

func TestBranchLimit(t *testing.T) {
	a, srv := testApp(t)
	a.db.Exec(`UPDATE businesses SET max_branches=2`)
	owner := demoOwner(t, srv)
	addGulshan(t, a, owner)
	owner.do("POST", "/owner/branches", url.Values{"branch_name": {"Banani"}})
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM restaurants`).Scan(&n)
	if n != 2 {
		t.Fatalf("%d branches past a limit of 2", n)
	}
	if !strings.Contains(owner.get("/owner/branches?err=x"), "2 of 2 in use") {
		t.Error("Branches page does not show the limit")
	}
}

func TestArchivedAndClosedBranches(t *testing.T) {
	a, srv := testApp(t)
	owner := demoOwner(t, srv)
	gul := addGulshan(t, a, owner)
	switchTo(owner, gul)
	owner.do("POST", "/owner/tables", url.Values{"label": {"1"}})
	owner.do("POST", "/owner/staff", url.Values{"name": {"Sumi"}, "phone": {"01830000000"}, "password": {"pass12345"}, "role": {"cashier"}})
	var tok string
	a.db.QueryRow(`SELECT qr_token FROM tables WHERE restaurant_id=?`, gul).Scan(&tok)
	if tok == "" {
		t.Fatal("no table in Gulshan")
	}

	// Closed for now: guests see it and can't order; staff still log in.
	owner.do("POST", fmt.Sprintf("/owner/branches/%d/status", gul), url.Values{"status": {"closed"}})
	g := newTClient(t, srv)
	if page := g.get("/t/" + tok); !strings.Contains(page, "Temporarily closed") && !strings.Contains(page, "সাময়িকভাবে বন্ধ") {
		t.Error("guest of a closed branch is not told it is closed")
	}
	cashier := newTClient(t, srv)
	cashier.c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if code, _ := cashier.do("POST", "/login", url.Values{"phone": {"01830000000"}, "password": {"pass12345"}}); code != http.StatusSeeOther {
		t.Errorf("staff of a closed branch can't log in: %d", code)
	}

	// Archiving with an open bill is refused.
	g.do("POST", "/t/"+tok+"/enter", url.Values{"name": {"Rafi"}, "choice": {"new"}})
	owner.do("POST", fmt.Sprintf("/owner/branches/%d/status", gul), url.Values{"status": {"archived"}})
	var status string
	a.db.QueryRow(`SELECT status FROM restaurants WHERE id=?`, gul).Scan(&status)
	if status != "closed" {
		t.Fatalf("archived with an open bill: %s", status)
	}
	a.db.Exec(`UPDATE table_sessions SET status='closed' WHERE restaurant_id=?`, gul)
	owner.do("POST", fmt.Sprintf("/owner/branches/%d/status", gul), url.Values{"status": {"archived"}})
	a.db.QueryRow(`SELECT status FROM restaurants WHERE id=?`, gul).Scan(&status)
	if status != "archived" {
		t.Fatalf("not archived: %s", status)
	}
	if code, _ := newTClient(t, srv).do("GET", "/t/"+tok, nil); code != http.StatusNotFound {
		t.Errorf("QR of an archived branch opens: %d", code)
	}
	if code, _ := cashier.do("GET", "/staff/counter", nil); code != http.StatusSeeOther {
		t.Errorf("staff of an archived branch still logged in: %d", code)
	}
	if code, _ := cashier.do("POST", "/login", url.Values{"phone": {"01830000000"}, "password": {"pass12345"}}); code == http.StatusSeeOther {
		t.Error("staff of an archived branch can log in")
	}
	// The only branch left in use can't be archived.
	owner.do("POST", fmt.Sprintf("/owner/branches/%d/status", demoID(a)), url.Values{"status": {"archived"}})
	a.db.QueryRow(`SELECT status FROM restaurants WHERE id=?`, demoID(a)).Scan(&status)
	if status != "active" {
		t.Errorf("the last branch in use was archived")
	}
}

func TestEachBranchCountsItsOwnInvoices(t *testing.T) {
	a, srv := testApp(t)
	owner := demoOwner(t, srv)
	gul := addGulshan(t, a, owner)
	switchTo(owner, gul)
	owner.do("POST", "/owner/tables", url.Values{"label": {"1"}})
	var tok string
	a.db.QueryRow(`SELECT qr_token FROM tables WHERE restaurant_id=?`, gul).Scan(&tok)

	pay := func(tok string) int64 { return payBorhani(t, a, srv, owner, tok) }
	gulBill := pay(tok)
	switchTo(owner, demoID(a))
	demoBill := pay("demo-table-1")
	var gulNo, demoNo int64
	a.db.QueryRow(`SELECT invoice_no FROM table_sessions WHERE id=?`, gulBill).Scan(&gulNo)
	a.db.QueryRow(`SELECT invoice_no FROM table_sessions WHERE id=?`, demoBill).Scan(&demoNo)
	if gulNo != 1 || demoNo != 1 {
		t.Fatalf("invoice numbers: Gulshan %d, Demo Kitchen %d, want 1 and 1", gulNo, demoNo)
	}
	switchTo(owner, gul)
	if slip := owner.get(fmt.Sprintf("/staff/sessions/%d/print", gulBill)); !strings.Contains(slip, "GUL-0001") || !strings.Contains(slip, "Demo Kitchen · Gulshan") {
		t.Error("Gulshan's printed bill does not show GUL-0001 and the branch name")
	}
}

// payBorhani has a guest at table tok order one Borhani, and the owner (working in that branch)
// take cash for it. It returns the bill's id.
func payBorhani(t *testing.T, a *App, srv *httptest.Server, owner *tclient, tok string) int64 {
	t.Helper()
	g := newTClient(t, srv)
	g.get("/t/" + tok)
	g.do("POST", "/t/"+tok+"/enter", url.Values{"name": {"Nila"}, "choice": {"new"}})
	var item int64
	a.db.QueryRow(`SELECT m.id FROM menu_items m JOIN tables t ON t.restaurant_id=m.restaurant_id WHERE t.qr_token=? AND m.name_en='Borhani'`, tok).Scan(&item)
	g.post("/t/"+tok+"/cart/add", url.Values{"item_id": {fmt.Sprint(item)}, "from": {"card"}})
	g.do("POST", "/t/"+tok+"/order", url.Values{})
	var sid int64
	a.db.QueryRow(`SELECT session_id FROM orders ORDER BY id DESC LIMIT 1`).Scan(&sid)
	owner.do("POST", fmt.Sprintf("/staff/sessions/%d/paid", sid), url.Values{"method": {"cash"}})
	var paid bool
	a.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM payments WHERE session_id=?)`, sid).Scan(&paid)
	if !paid {
		t.Fatalf("bill %d at %s not paid", sid, tok)
	}
	return sid
}

func TestAllBranchesViews(t *testing.T) {
	a, srv := testApp(t)
	owner := demoOwner(t, srv)
	gul := addGulshan(t, a, owner)
	switchTo(owner, gul)
	owner.do("POST", "/owner/tables", url.Values{"label": {"1"}})
	var tok string
	a.db.QueryRow(`SELECT qr_token FROM tables WHERE restaurant_id=?`, gul).Scan(&tok)
	payBorhani(t, a, srv, owner, tok)
	switchTo(owner, demoID(a))
	payBorhani(t, a, srv, owner, "demo-table-1")
	payBorhani(t, a, srv, owner, "demo-table-2")

	var each, all int64
	a.db.QueryRow(`SELECT SUM(amount) FROM payments WHERE restaurant_id=?`, gul).Scan(&each)
	a.db.QueryRow(`SELECT SUM(amount) FROM payments`).Scan(&all)
	switchTo(owner, "all")
	closing := owner.get("/owner/closing")
	if !strings.Contains(closing, money.Format(all)) || !strings.Contains(closing, money.Format(each)) || !strings.Contains(closing, "By branch") {
		t.Errorf("all-branches closing report does not show the total %s and Gulshan's %s", money.Format(all), money.Format(each))
	}
	if dash := owner.get("/owner"); !strings.Contains(dash, "all branches") || !strings.Contains(dash, money.Format(all)) || !strings.Contains(dash, "Gulshan") {
		t.Error("all-branches dashboard does not show the total and each branch")
	}
	if !strings.Contains(owner.get("/owner/activity"), `<span class="badge">Gulshan</span>`) {
		t.Error("all-branches activity log has no branch column")
	}
	csv := owner.get("/owner/export/payments.csv")
	if strings.Count(csv, "\n") != 4 || !strings.Contains(csv, "Gulshan,GUL-0001") || !strings.Contains(csv, "Demo Kitchen,2") {
		t.Errorf("all-branches payments CSV:\n%s", csv)
	}
	// One branch: only its own payments.
	switchTo(owner, gul)
	if csv := owner.get("/owner/export/payments.csv"); strings.Count(csv, "\n") != 2 {
		t.Errorf("Gulshan's payments CSV has other branches:\n%s", csv)
	}
}

// A branch shows the business's logo, colour and tagline unless it has its own (multi-branch.md, 8.3).
func TestBranchBranding(t *testing.T) {
	a, srv := testApp(t)
	owner := demoOwner(t, srv)
	gul := addGulshan(t, a, owner)
	color := func(rid int64) string {
		var x Restaurant
		if err := a.db.QueryRow(`SELECT `+restCols+` FROM restaurants r WHERE r.id=?`, rid).Scan(x.dest()...); err != nil {
			t.Fatal(err)
		}
		return x.Color
	}
	// Business colour reaches both branches.
	owner.do("POST", "/owner/business", url.Values{"name": {"Demo Kitchen"}, "color": {"#1d4ed8"}, "tagline": {"Fresh"}})
	if color(demoID(a)) != "#1d4ed8" || color(gul) != "#1d4ed8" {
		t.Fatalf("business colour: demo %s, Gulshan %s", color(demoID(a)), color(gul))
	}
	// Gulshan's own colour wins there only, and "use the business's" takes it back.
	switchTo(owner, gul)
	owner.do("POST", "/owner/profile", url.Values{"color": {"#15803d"}, "tagline": {"Gulshan"}})
	if color(gul) != "#15803d" || color(demoID(a)) != "#1d4ed8" {
		t.Fatalf("branch colour: demo %s, Gulshan %s", color(demoID(a)), color(gul))
	}
	owner.do("POST", "/owner/profile", url.Values{"use_business": {"1"}})
	if color(gul) != "#1d4ed8" {
		t.Fatalf("Gulshan after 'use the business's': %s", color(gul))
	}
	// A manager edits the branch details, not its branding.
	owner.do("POST", "/owner/staff", url.Values{"name": {"Mina"}, "phone": {"01840000001"}, "password": {"pass12345"}, "role": {"manager"}})
	m := newTClient(t, srv)
	m.do("POST", "/login", url.Values{"phone": {"01840000001"}, "password": {"pass12345"}})
	m.do("POST", "/owner/profile", url.Values{"color": {"#15803d"}, "address": {"Road 11, Gulshan"}})
	var addr string
	a.db.QueryRow(`SELECT address FROM restaurants WHERE id=?`, gul).Scan(&addr)
	if color(gul) != "#1d4ed8" || addr != "Road 11, Gulshan" {
		t.Errorf("manager: colour %s, address %q", color(gul), addr)
	}
}

// DEMO=1 shows branches: Dhanmondi and Gulshan, each with its own menu, and the owner starts in
// "All branches".
func TestDemoHasTwoBranches(t *testing.T) {
	a, srv := testApp(t)
	a.cfg.DemoBranches = true
	a.seedDemoBranches()
	a.seedDemoBranches() // a restart does not add a third
	var n, dishes int
	a.db.QueryRow(`SELECT COUNT(*) FROM restaurants`).Scan(&n)
	a.db.QueryRow(`SELECT COUNT(*) FROM menu_items m JOIN restaurants r ON r.id=m.restaurant_id WHERE r.slug='demo-gulshan'`).Scan(&dishes)
	if n != 2 || dishes == 0 {
		t.Fatalf("%d branches, Gulshan has %d dishes", n, dishes)
	}
	owner := demoOwner(t, srv)
	if dash := owner.get("/owner"); !strings.Contains(dash, "all branches") || !strings.Contains(dash, "Dhanmondi") || !strings.Contains(dash, "Gulshan") {
		t.Error("the demo owner does not start in All branches with both branches")
	}
	if page := newTClient(t, srv).get("/t/demo-gulshan-1"); !strings.Contains(page, "Demo Kitchen · Gulshan") {
		t.Error("Gulshan's table does not show the branch name")
	}
}
