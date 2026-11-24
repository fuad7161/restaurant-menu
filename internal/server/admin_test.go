package server

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// adminLogin logs c in as the admin: password, then the code from the email.
func adminLogin(t *testing.T, c *tclient, fm *fakeMail, email, pass string) {
	t.Helper()
	c.do("POST", "/admin/login", url.Values{"email": {email}, "password": {pass}})
	fm.mu.Lock()
	if len(fm.sent) == 0 {
		fm.mu.Unlock()
		t.Fatal("no admin code emailed")
	}
	last := fm.sent[len(fm.sent)-1]
	fm.mu.Unlock()
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(last.Text)
	if last.To != email || code == "" {
		t.Fatalf("code email to %s: %q", last.To, last.Text)
	}
	c.do("POST", "/admin/code", url.Values{"code": {code}})
}

func TestAdminLoginNeedsTheEmailedCode(t *testing.T) {
	a, fm, c := mailApp(t)
	if err := CreateAdmin(a.db, "admin@example.com", "short"); err == nil {
		t.Error("an admin was made with a short password")
	}
	if err := CreateAdmin(a.db, "admin@example.com", "a-long-admin-password"); err != nil {
		t.Fatal(err)
	}
	c.c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// A restaurant owner's login does not open the admin panel.
	c.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	if code, _ := c.do("GET", "/admin", nil); code != http.StatusSeeOther {
		t.Fatalf("owner opened /admin: %d", code)
	}
	// The password alone is not enough, and a wrong code is refused.
	c.do("POST", "/admin/login", url.Values{"email": {"admin@example.com"}, "password": {"a-long-admin-password"}})
	if code, _ := c.do("GET", "/admin", nil); code != http.StatusSeeOther {
		t.Fatal("admin panel open after the password only")
	}
	if _, body := c.do("POST", "/admin/code", url.Values{"code": {"000000"}}); !strings.Contains(body, "not right") && !strings.Contains(body, "expired") {
		t.Error("a wrong code was not refused")
	}
	// Five wrong codes end the try.
	for i := 0; i < 5; i++ {
		c.do("POST", "/admin/code", url.Values{"code": {"000000"}})
	}
	fm.mu.Lock()
	code := regexp.MustCompile(`\b\d{6}\b`).FindString(fm.sent[len(fm.sent)-1].Text)
	fm.mu.Unlock()
	c.do("POST", "/admin/code", url.Values{"code": {code}})
	if code, _ := c.do("GET", "/admin", nil); code != http.StatusSeeOther {
		t.Fatal("the right code worked after five wrong ones")
	}

	adminLogin(t, c, fm, "admin@example.com", "a-long-admin-password")
	if code, body := c.do("GET", "/admin", nil); code != http.StatusOK || !strings.Contains(body, "Demo Kitchen") {
		t.Fatalf("admin home: %d", code)
	}
}

func TestAdminSetsBranchLimitAndVerification(t *testing.T) {
	a, fm, c := mailApp(t)
	CreateAdmin(a.db, "admin@example.com", "a-long-admin-password")
	adminLogin(t, c, fm, "admin@example.com", "a-long-admin-password")
	var bid int64
	a.db.QueryRow(`SELECT business_id FROM restaurants WHERE slug='demo'`).Scan(&bid)
	page := fmt.Sprintf("/admin/businesses/%d", bid)

	c.do("POST", page+"/branches", url.Values{"max_branches": {"0"}})
	c.do("POST", page+"/branches", url.Values{"max_branches": {"5"}})
	c.do("POST", page+"/verify", url.Values{"verify": {"off"}})
	var max int
	var verify string
	a.db.QueryRow(`SELECT max_branches FROM businesses WHERE id=?`, bid).Scan(&max)
	a.db.QueryRow(`SELECT verify_email FROM restaurants WHERE business_id=?`, bid).Scan(&verify)
	if max != 5 || verify != "off" {
		t.Fatalf("branch limit %d, verification %s; want 5 and off", max, verify)
	}
	if body := c.get(page); !strings.Contains(body, "3 → 5") || !strings.Contains(body, "global → off") {
		t.Error("the admin log does not show the changes with before and after")
	}

	// The owner now gets 5 branches, and a new branch keeps the business's verification setting.
	jar, _ := cookiejar.New(nil)
	owner := &tclient{t, &http.Client{Jar: jar}, c.url}
	owner.do("POST", "/login", url.Values{"phone": {"01700000000"}, "password": {"demo1234"}})
	for _, n := range []string{"B1", "B2", "B3", "B4", "B5"} {
		owner.do("POST", "/owner/branches", url.Values{"branch_name": {n}})
	}
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM restaurants WHERE business_id=? AND verify_email='off'`, bid).Scan(&n)
	if n != 5 {
		t.Errorf("%d branches with verification off, want 5 (the limit)", n)
	}

	// A restaurant login can't change the limit.
	owner.do("POST", page+"/branches", url.Values{"max_branches": {"50"}})
	a.db.QueryRow(`SELECT max_branches FROM businesses WHERE id=?`, bid).Scan(&max)
	if max != 5 {
		t.Error("an owner changed the branch limit")
	}
}
