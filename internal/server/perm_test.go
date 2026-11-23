package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"bosho/internal/database"
)

// routeAccess is the route table of docs/plans/owner-verification-admin-roles.md section 7:
// which roles may use each route. Ids that don't exist are fine: a refused role gets 403 before
// the id is looked at, an allowed one gets anything else (often 404).
var routeAccess = []struct {
	method, path string
	allowed      string // roles, space separated
}{
	{"GET", "/staff/account", "owner kitchen waiter cashier"},
	{"POST", "/staff/account/password", "owner"},
	{"POST", "/staff/account/email", "owner"},
	{"POST", "/staff/account/email/resend", "owner"},
	{"GET", "/staff/kitchen", "owner kitchen waiter"},
	{"GET", "/staff/kitchen/board", "owner kitchen waiter"},
	{"POST", "/staff/orders/999999/accept", "owner kitchen"},
	{"POST", "/staff/orders/999999/reject", "owner kitchen"},
	{"POST", "/staff/orders/999999/ready", "owner kitchen"},
	{"POST", "/staff/orders/999999/served", "owner waiter"},
	{"POST", "/staff/orders/999999/undo", "owner kitchen waiter"}, // each only its own step
	{"POST", "/staff/items/999999/approve", "owner kitchen waiter"},
	{"POST", "/staff/items/999999/decline", "owner kitchen waiter"},
	{"POST", "/staff/items/999999/void", "owner"},
	{"POST", "/staff/calls/999999/done", "owner waiter"},
	{"GET", "/staff/stock", "owner kitchen waiter"},
	{"POST", "/staff/stock/999999/toggle", "owner kitchen waiter"},
	{"GET", "/staff/counter", "owner cashier"},
	{"GET", "/staff/counter/list", "owner cashier"},
	{"GET", "/staff/sessions/999999", "owner cashier waiter"},
	{"POST", "/staff/sessions/999999/paid", "owner cashier"},
	{"POST", "/staff/sessions/999999/close", "owner"},
	{"GET", "/staff/tables", "owner waiter"},
	{"POST", "/staff/tables/999999/open", "owner waiter"},
	{"GET", "/owner", "owner"},
	{"GET", "/owner/menu", "owner"},
	{"POST", "/owner/items/999999/delete", "owner"},
	{"GET", "/owner/tables", "owner"},
	{"POST", "/owner/tables/999999/newqr", "owner"},
	{"GET", "/owner/staff", "owner"},
	{"POST", "/owner/staff/999999/password", "owner"},
	{"GET", "/owner/activity", "owner"},
	{"GET", "/owner/reports", "owner"},
	{"GET", "/owner/export/payments.csv", "owner"},
	{"GET", "/owner/closing", "owner cashier"},
	{"GET", "/owner/settings", "owner"},
	{"POST", "/owner/settings", "owner"},
	{"GET", "/owner/networks", "owner"},
	{"GET", "/owner/profile", "owner"},
}

func TestRolesOnlyDoTheirOwnTasks(t *testing.T) {
	_, srv := testApp(t)
	owner := newTClient(t, srv)
	owner.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	clients := map[string]*tclient{"owner": owner}
	for i, role := range []string{"kitchen", "waiter", "cashier"} {
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
