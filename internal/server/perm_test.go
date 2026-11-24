package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"bosho/internal/database"

	"golang.org/x/crypto/bcrypt"
)

// routeAccess is the route table of docs/plans/owner-verification-admin-roles.md section 7:
// which roles may use each route. Ids that don't exist are fine: a refused role gets 403 before
// the id is looked at, an allowed one gets anything else (often 404).
var routeAccess = []struct {
	method, path string
	allowed      string // roles, space separated
}{
	{"GET", "/staff/account", "owner manager kitchen waiter cashier"},
	{"POST", "/staff/account/password", "owner"},
	{"POST", "/staff/account/email", "owner"},
	{"POST", "/staff/account/email/resend", "owner"},
	{"GET", "/staff/kitchen", "owner manager kitchen waiter"},
	{"GET", "/staff/kitchen/board", "owner manager kitchen waiter"},
	{"POST", "/staff/orders/999999/accept", "owner manager kitchen"},
	{"POST", "/staff/orders/999999/reject", "owner manager kitchen"},
	{"POST", "/staff/orders/999999/ready", "owner manager kitchen"},
	{"POST", "/staff/orders/999999/served", "owner manager waiter"},
	{"POST", "/staff/orders/999999/undo", "owner manager kitchen waiter"}, // each only its own step
	{"POST", "/staff/items/999999/approve", "owner manager kitchen waiter"},
	{"POST", "/staff/items/999999/decline", "owner manager kitchen waiter"},
	{"POST", "/staff/items/999999/void", "owner manager"},
	{"POST", "/staff/calls/999999/done", "owner manager waiter"},
	{"GET", "/staff/stock", "owner manager kitchen waiter"},
	{"POST", "/staff/stock/999999/toggle", "owner manager kitchen waiter"},
	{"GET", "/staff/counter", "owner manager cashier"},
	{"GET", "/staff/counter/list", "owner manager cashier"},
	{"GET", "/staff/sessions/999999", "owner manager cashier waiter"},
	{"POST", "/staff/sessions/999999/paid", "owner manager cashier"},
	{"POST", "/staff/sessions/999999/discount", "owner manager"}, // owner and manager only (multi-branch.md, B5)
	{"GET", "/staff/sessions/999999/print", "owner manager cashier waiter"},
	{"POST", "/staff/sessions/999999/close", "owner manager"},
	{"GET", "/staff/tables", "owner manager waiter"},
	{"POST", "/staff/tables/999999/open", "owner manager waiter"},
	{"GET", "/owner", "owner manager"},
	{"GET", "/owner/menu", "owner manager"},
	{"POST", "/owner/items/999999/delete", "owner manager"},
	{"GET", "/owner/tables", "owner manager"},
	{"POST", "/owner/tables/999999/newqr", "owner manager"},
	{"GET", "/owner/staff", "owner manager"},
	{"POST", "/owner/staff/999999/password", "owner manager"},
	{"GET", "/owner/activity", "owner manager"},
	{"GET", "/owner/reports", "owner manager"},
	{"GET", "/owner/export/payments.csv", "owner manager"},
	{"GET", "/owner/closing", "owner manager cashier"},
	{"GET", "/owner/settings", "owner manager"},
	{"POST", "/owner/settings", "owner manager"},
	{"GET", "/owner/networks", "owner manager"},
	{"GET", "/owner/profile", "owner manager"},
	{"GET", "/owner/branches", "owner"},
	{"POST", "/owner/branches", "owner"},
	{"POST", "/owner/branches/999999", "owner"},
	{"POST", "/owner/branches/999999/status", "owner"},
	{"POST", "/owner/branch", "owner"},
	{"POST", "/owner/staff/999999/move", "owner"},
	{"GET", "/owner/business", "owner"},
	{"POST", "/owner/business", "owner"},
	{"POST", "/owner/profile/logo/delete", "owner"},
}

func TestRolesOnlyDoTheirOwnTasks(t *testing.T) {
	_, srv := testApp(t)
	owner := newTClient(t, srv)
	owner.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	clients := map[string]*tclient{"owner": owner}
	for i, role := range []string{"kitchen", "waiter", "cashier", "manager"} {
		phone := "0181000000" + string(rune('1'+i))
		owner.do("POST", "/owner/staff", url.Values{"name": {role}, "phone": {phone}, "password": {"pass12345"}, "role": {role}})
		c := newTClient(t, srv)
		c.c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		if code, _ := c.do("POST", "/login", url.Values{"phone": {phone}, "password": {"pass12345"}}); code != http.StatusSeeOther {
			t.Fatalf("%s login: status %d", role, code)
		}
		clients[role] = c
	}
	for _, rt := range routeAccess {
		for role, c := range clients {
			var form url.Values
			if rt.method == "POST" {
				form = url.Values{"password": {"pass12345"}}
			}
			code, _ := c.do(rt.method, rt.path, form, "HX-Request", "true")
			want := strings.Contains(" "+rt.allowed+" ", " "+role+" ")
			if got := code != http.StatusForbidden; got != want {
				t.Errorf("%s %s as %s: status %d, allowed=%v", rt.method, rt.path, role, code, want)
			}
			// An allowed page must also render for that role (its buttons depend on the role).
			if want && rt.method == "GET" && !strings.Contains(rt.path, "999999") && code != http.StatusOK {
				t.Errorf("%s %s as %s: status %d, want 200", rt.method, rt.path, role, code)
			}
		}
	}
}

func TestOwnerCannotCreateOrRemoveOwners(t *testing.T) {
	a, srv := testApp(t)
	c := newTClient(t, srv)
	c.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	c.do("POST", "/owner/staff", url.Values{"name": {"Second"}, "phone": {"01811111111"}, "password": {"pass12345"}, "role": {"owner"}})
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM staff_users WHERE phone='01811111111'`).Scan(&n)
	if n != 0 {
		t.Fatal("an owner could add another owner")
	}
	// An owner that already exists (from before this rule) can't be removed or have their
	// password set by the other owner.
	var rid int64
	a.db.QueryRow(`SELECT restaurant_id FROM staff_users WHERE phone='01700000000'`).Scan(&rid)
	other, err := database.InsertID(a.db, `INSERT INTO staff_users(restaurant_id,name,phone,password_hash,role) VALUES(?,'Old owner','01822222222','x','owner')`, rid)
	if err != nil {
		t.Fatal(err)
	}
	c.do("POST", "/owner/staff/"+fmt.Sprint(other)+"/delete", url.Values{})
	c.do("POST", "/owner/staff/"+fmt.Sprint(other)+"/password", url.Values{"password": {"takeover123"}})
	var hash string
	if err := a.db.QueryRow(`SELECT password_hash FROM staff_users WHERE id=?`, other).Scan(&hash); err != nil {
		t.Fatal("one owner removed another owner")
	}
	if hash != "x" {
		t.Fatal("one owner set another owner's password")
	}
}

func TestEveryRoleHasAStartPageItCanOpen(t *testing.T) {
	start := map[string]string{"/owner": PermDashboard, "/staff/counter": PermCounterView, "/staff/tables": PermTablesOrder, "/staff/kitchen": PermKitchenView}
	for role := range rolePerms {
		s := &Staff{Role: role}
		if !s.Can(start[s.Home()]) {
			t.Errorf("%s starts on %s but can't open it", role, s.Home())
		}
	}
}

// A manager runs their branch's staff, but not managers or the owner, and can't change VAT or
// service charge (docs/plans/multi-branch.md, section 3).
func TestManagerLimits(t *testing.T) {
	a, srv := testApp(t)
	owner := newTClient(t, srv)
	owner.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	owner.do("POST", "/owner/staff", url.Values{"name": {"Mina"}, "phone": {"01840000001"}, "password": {"pass12345"}, "role": {"manager"}})
	owner.do("POST", "/owner/staff", url.Values{"name": {"Rana"}, "phone": {"01840000002"}, "password": {"other1234"}, "role": {"manager"}})
	id := func(phone string) string {
		var id int64
		a.db.QueryRow(`SELECT id FROM staff_users WHERE phone=?`, phone).Scan(&id)
		return fmt.Sprint(id)
	}
	if id("01840000002") == "0" {
		t.Fatal("the owner could not add a manager")
	}
	m := newTClient(t, srv)
	m.do("POST", "/login", url.Values{"phone": {"01840000001"}, "password": {"pass12345"}})

	m.do("POST", "/owner/staff", url.Values{"name": {"Third"}, "phone": {"01840000003"}, "password": {"pass12345"}, "role": {"manager"}})
	m.do("POST", "/owner/staff", url.Values{"name": {"Kalam"}, "phone": {"01840000004"}, "password": {"pass12345"}, "role": {"waiter"}})
	if id("01840000003") != "0" || id("01840000004") == "0" {
		t.Error("a manager can add a manager, or can't add a waiter")
	}
	for _, phone := range []string{"01840000002", "01700000000"} {
		m.do("POST", "/owner/staff/"+id(phone)+"/password", url.Values{"password": {"takeover123"}})
		m.do("POST", "/owner/staff/"+id(phone)+"/delete", url.Values{})
		var hash string
		if err := a.db.QueryRow(`SELECT password_hash FROM staff_users WHERE phone=?`, phone).Scan(&hash); err != nil {
			t.Fatalf("a manager removed %s", phone)
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte("takeover123")) == nil {
			t.Fatalf("a manager set the password of %s", phone)
		}
	}
	if page := m.get("/owner/staff"); strings.Contains(page, "other1234") || !strings.Contains(page, "Kalam") {
		t.Error("the staff page shows another manager's password, or not the waiter")
	}
	m.do("POST", "/owner/settings", url.Values{"vat": {"0"}, "service": {"0"}, "is_open": {"1"}})
	var vat, svc float64
	a.db.QueryRow(`SELECT vat_percent,service_percent FROM restaurants`).Scan(&vat, &svc)
	if vat != 5 || svc != 10 {
		t.Errorf("a manager changed VAT to %v and service to %v", vat, svc)
	}
}
