package server

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"bosho/internal/config"
	"bosho/internal/database"
)

// These tests run the real app against PostgreSQL. They need TEST_DATABASE_URL, for example a
// local Docker PostgreSQL; without it they are skipped. They work in a new schema of their own
// and drop it afterwards, so existing data in that database is never touched.
func testApp(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	schema := "bosho_test_" + randToken(4)
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		admin.Close()
	})
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := database.Open(dsn + sep + "search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	a, err := New(config.Config{Demo: true}, db, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return a, srv
}

type tclient struct {
	t   *testing.T
	c   *http.Client
	url string
}

func newTClient(t *testing.T, srv *httptest.Server) *tclient {
	jar, _ := cookiejar.New(nil)
	return &tclient{t, &http.Client{Jar: jar}, srv.URL}
}

func (c *tclient) do(method, path string, form url.Values, hdr ...string) (int, string) {
	c.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, _ := http.NewRequest(method, c.url+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (c *tclient) get(path string) string { _, b := c.do("GET", path, nil); return b }
func (c *tclient) post(path string, form url.Values) string {
	_, b := c.do("POST", path, form, "HX-Request", "true")
	return b
}

func itemID(t *testing.T, a *App, en string) string {
	var id int64
	if err := a.db.QueryRow(`SELECT id FROM menu_items WHERE name_en=?`, en).Scan(&id); err != nil {
		t.Fatalf("dish %s: %v", en, err)
	}
	return fmt.Sprint(id)
}

func TestOrderBillAndPayment(t *testing.T) {
	a, srv := testApp(t)
	g := newTClient(t, srv)
	g.get("/t/demo-table-1")
	g.do("POST", "/t/demo-table-1/enter", url.Values{"name": {"Rafi"}, "choice": {"new"}})
	// Kacchi has extras, so it is added from its dish sheet; a card cannot add it.
	if code, _ := g.do("POST", "/t/demo-table-1/cart/add", url.Values{"item_id": {itemID(t, a, "Kacchi Biryani")}, "from": {"card"}}, "HX-Request", "true"); code != http.StatusBadRequest {
		t.Errorf("card add of a dish with options: %d, want 400", code)
	}
	g.post("/t/demo-table-1/cart/add", url.Values{"item_id": {itemID(t, a, "Kacchi Biryani")}, "from": {"sheet"}, "qty": {"1"}})
	g.post("/t/demo-table-1/cart/add", url.Values{"item_id": {itemID(t, a, "Borhani")}, "from": {"card"}})

	// Demo charges: service 10%, VAT 5%. 320 + 60 = 380, + 38 + 19 = 437.
	cart := g.get("/t/demo-table-1/cart")
	for _, want := range []string{"৳380", "৳38", "৳19", "৳437"} {
		if !strings.Contains(cart, want) {
			t.Errorf("cart does not show %s", want)
		}
	}
	g.do("POST", "/t/demo-table-1/order", url.Values{})
	var oid, sid int64
	a.db.QueryRow(`SELECT id,session_id FROM orders ORDER BY id DESC LIMIT 1`).Scan(&oid, &sid)
	if oid == 0 {
		t.Fatal("no order placed")
	}
	var lines int
	a.db.QueryRow(`SELECT COUNT(*) FROM order_items WHERE order_id=?`, oid).Scan(&lines)
	if lines != 2 {
		t.Fatalf("order has %d lines, want 2", lines)
	}

	staff := newTClient(t, srv)
	staff.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	staff.post(fmt.Sprintf("/staff/orders/%d/accept", oid), url.Values{"minutes": {"20"}})

	// After the kitchen accepts, cancelling is a request that staff approve.
	var borhani int64
	a.db.QueryRow(`SELECT id FROM order_items WHERE order_id=? AND name_en='Borhani'`, oid).Scan(&borhani)
	g.post(fmt.Sprintf("/t/demo-table-1/items/%d/change", borhani), url.Values{"action": {"cancel"}})
	var st string
	a.db.QueryRow(`SELECT status FROM order_items WHERE id=?`, borhani).Scan(&st)
	if st != "cancel_requested" {
		t.Fatalf("item status %q, want cancel_requested", st)
	}
	staff.post(fmt.Sprintf("/staff/items/%d/approve", borhani), url.Values{})
	a.db.QueryRow(`SELECT status FROM order_items WHERE id=?`, borhani).Scan(&st)
	if st != "cancelled" {
		t.Fatalf("item status %q after approval, want cancelled", st)
	}

	// Bill is now 320 + 32 + 16 = 368. Paying twice records one payment.
	staff.do("POST", fmt.Sprintf("/staff/sessions/%d/paid", sid), url.Values{"method": {"bkash"}})
	staff.do("POST", fmt.Sprintf("/staff/sessions/%d/paid", sid), url.Values{"method": {"cash"}})
	var n int
	var amount int64
	a.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(amount),0) FROM payments WHERE session_id=?`, sid).Scan(&n, &amount)
	if n != 1 || amount != 36800 {
		t.Fatalf("payments: %d totalling %d poisha, want 1 of 36800", n, amount)
	}
	a.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='bill paid'`).Scan(&n)
	if n != 1 {
		t.Errorf("audit log has %d 'bill paid' entries, want 1", n)
	}
	if !strings.Contains(g.get("/t/demo-table-1/receipt"), "৳368") {
		t.Error("receipt does not show the paid total")
	}
}

func TestJoinNeedsApproval(t *testing.T) {
	a, srv := testApp(t)
	first, second := newTClient(t, srv), newTClient(t, srv)
	first.get("/t/demo-table-2")
	first.do("POST", "/t/demo-table-2/enter", url.Values{"name": {"Rafi"}, "choice": {"new"}})
	var sid int64
	a.db.QueryRow(`SELECT id FROM table_sessions ORDER BY id DESC LIMIT 1`).Scan(&sid)
	second.get("/t/demo-table-2")
	second.do("POST", "/t/demo-table-2/enter", url.Values{"name": {"Karim"}, "choice": {fmt.Sprintf("join:%d", sid)}})
	var gid int64
	var st string
	a.db.QueryRow(`SELECT id,status FROM session_guests WHERE display_name='Karim'`).Scan(&gid, &st)
	if st != "pending" {
		t.Fatalf("joining guest is %q, want pending", st)
	}
	// The waiting guest cannot add to the cart.
	second.post("/t/demo-table-2/cart/add", url.Values{"item_id": {itemID(t, a, "Borhani")}, "from": {"card"}})
	var carts int
	a.db.QueryRow(`SELECT COUNT(*) FROM cart_items WHERE session_id=?`, sid).Scan(&carts)
	if carts != 0 {
		t.Fatal("a guest waiting for approval added to the cart")
	}
	first.post(fmt.Sprintf("/t/demo-table-2/join/%d/allow", gid), url.Values{})
	a.db.QueryRow(`SELECT status FROM session_guests WHERE id=?`, gid).Scan(&st)
	if st != "joined" {
		t.Fatalf("guest is %q after Allow, want joined", st)
	}
}

func TestLoginLockAndCrossSite(t *testing.T) {
	_, srv := testApp(t)
	c := newTClient(t, srv)
	for i := 0; i < loginTries; i++ {
		c.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"wrong"}})
	}
	if code, _ := c.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}}); code != http.StatusTooManyRequests {
		t.Errorf("login after %d wrong passwords: status %d, want 429", loginTries, code)
	}
	if code, _ := c.do("POST", "/t/demo-table-1/call", url.Values{"reason": {"water"}}, "Sec-Fetch-Site", "cross-site"); code != http.StatusForbidden {
		t.Errorf("cross-site post: status %d, want 403", code)
	}
	if code, _ := c.do("POST", "/t/demo-table-1/call", url.Values{}, "Origin", "https://evil.example"); code != http.StatusForbidden {
		t.Errorf("post from another origin: status %d, want 403", code)
	}
}

func TestWithinHours(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.UTC) }
	day := Restaurant{OpenTime: "12:00", CloseTime: "23:00", LastOrder: "22:30"}
	night := Restaurant{OpenTime: "18:00", CloseTime: "02:00"}
	cases := []struct {
		r    Restaurant
		at   time.Time
		want bool
	}{
		{day, at(11, 59), false}, {day, at(12, 0), true}, {day, at(22, 30), true}, {day, at(22, 31), false},
		{night, at(17, 0), false}, {night, at(23, 0), true}, {night, at(1, 59), true}, {night, at(2, 1), false},
		{Restaurant{}, at(3, 0), true},
	}
	for _, c := range cases {
		if got := withinHours(c.r, c.at); got != c.want {
			t.Errorf("%+v at %s: %v, want %v", c.r, c.at.Format("15:04"), got, c.want)
		}
	}
}

func TestReceiptAndExportNeedPayment(t *testing.T) {
	_, srv := testApp(t)
	staff := newTClient(t, srv)
	staff.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	code, body := staff.do("GET", "/owner/export/payments.csv", nil)
	if code != 200 || !strings.HasPrefix(body, "\xEF\xBB\xBFtime,table,group,amount,method,marked by") {
		t.Errorf("payments export: %d %q", code, body[:min(60, len(body))])
	}
	if code, _ := staff.do("GET", "/owner/export/secrets.csv", nil); code != 404 {
		t.Errorf("unknown export: %d, want 404", code)
	}
	if b := staff.get("/owner/closing"); !regexp.MustCompile(`Closing report`).MatchString(b) {
		t.Error("closing report does not render")
	}
}
