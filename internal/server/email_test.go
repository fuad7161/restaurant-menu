package server

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"bosho/internal/mail"
)

// fakeMail keeps the messages instead of sending them.
type fakeMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (f *fakeMail) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMail) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.sent) }

var linkRE = regexp.MustCompile(`https?://\S+?(/[a-z-]+\?token=[A-Za-z0-9_-]+)`)

// lastLink is the path of the link in the newest email, for example /reset?token=...
func (f *fakeMail) lastLink(t *testing.T) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		t.Fatal("no email sent")
	}
	m := linkRE.FindStringSubmatch(f.sent[len(f.sent)-1].Text)
	if m == nil {
		t.Fatalf("no link in email: %q", f.sent[len(f.sent)-1].Text)
	}
	return m[1]
}

func mailApp(t *testing.T) (*App, *fakeMail, *tclient) {
	a, srv := testApp(t)
	fm := &fakeMail{}
	a.mail = fm
	return a, fm, newTClient(t, srv)
}

func signupOwner(c *tclient) {
	c.do("POST", "/signup", url.Values{"restaurant": {"Kacchi Ghar"}, "name": {"Rahim"}, "phone": {"01910000000"},
		"email": {"rahim@example.com"}, "password": {"firstpass1"}})
}

func TestSignupWithVerificationOffSendsNothing(t *testing.T) {
	a, fm, c := mailApp(t)
	signupOwner(c)
	if fm.count() != 0 {
		t.Fatalf("%d emails sent while verification is off", fm.count())
	}
	a.db.Exec(`UPDATE restaurants SET verify_email='required'`)
	if body := c.get("/owner"); strings.Contains(body, "Confirm your email") {
		t.Fatal("the dashboard asks to confirm the email while verification is off")
	}
}

func TestSignupSendsAVerifyLinkThatWorksOnce(t *testing.T) {
	a, fm, c := mailApp(t)
	a.cfg.EmailVerify = true
	signupOwner(c)
	link := fm.lastLink(t)
	if !strings.HasPrefix(link, "/verify-email?") {
		t.Fatalf("first email links to %s", link)
	}
	if body := c.get(link); !strings.Contains(body, "Email confirmed") {
		t.Fatal("verify link did not confirm the email")
	}
	var at int64
	a.db.QueryRow(`SELECT email_verified_at FROM staff_users WHERE phone='01910000000'`).Scan(&at)
	if at == 0 {
		t.Fatal("email_verified_at not set")
	}
	if body := c.get(link); !strings.Contains(body, "doesn't work") {
		t.Fatal("a used verify link worked again")
	}
}

func TestForgotPasswordByEmail(t *testing.T) {
	_, fm, c := mailApp(t)
	signupOwner(c)
	before := fm.count()

	// Unknown accounts and staff get the same answer, and no email.
	for _, who := range []string{"01999999999", "nobody@example.com"} {
		if body := c.post("/forgot", url.Values{"who": {who}}); !strings.Contains(body, "Check your email") {
			t.Fatalf("forgot for %s: unexpected page", who)
		}
	}
	if fm.count() != before {
		t.Fatal("an email went out for an unknown account")
	}

	c.post("/forgot", url.Values{"who": {"RAHIM@example.com"}})
	link := fm.lastLink(t)
	if !strings.HasPrefix(link, "/reset?") {
		t.Fatalf("reset email links to %s", link)
	}
	tok := strings.TrimPrefix(link, "/reset?token=")
	if code, _ := c.do("POST", "/reset", url.Values{"token": {tok}, "password": {"secondpass2"}}); code != http.StatusOK && code != http.StatusSeeOther {
		t.Fatalf("reset: status %d", code)
	}
	// The reset logged every device out, this one included.
	if body := c.get("/owner"); !strings.Contains(body, "Staff login") {
		t.Fatal("still logged in after a password reset")
	}
	if body := c.get(link); !strings.Contains(body, "doesn't work") {
		t.Fatal("a used reset link still works")
	}
	jar, _ := cookiejar.New(nil)
	fresh := &tclient{t, &http.Client{Jar: jar}, c.url}
	if _, body := fresh.do("POST", "/login", url.Values{"phone": {"01910000000"}, "password": {"firstpass1"}}); !strings.Contains(body, "Wrong phone number or password") {
		t.Fatal("the old password still works")
	}
	fresh.do("POST", "/login", url.Values{"phone": {"01910000000"}, "password": {"secondpass2"}})
	if body := fresh.get("/owner"); !strings.Contains(body, "Today") {
		t.Fatal("the new password does not work")
	}
}

func TestStaffGetNoResetEmail(t *testing.T) {
	_, fm, c := mailApp(t)
	signupOwner(c)
	c.do("POST", "/owner/staff", url.Values{"name": {"Karim"}, "phone": {"01920000000"}, "password": {"waiterpass"}, "role": {"waiter"}})
	before := fm.count()
	c.post("/forgot", url.Values{"who": {"01920000000"}})
	if fm.count() != before {
		t.Fatal("a staff member got a reset email; their owner sets their password")
	}
}

func TestOwnerSeesStaffPasswords(t *testing.T) {
	_, _, c := mailApp(t)
	signupOwner(c)
	c.do("POST", "/owner/staff", url.Values{"name": {"Karim"}, "phone": {"01920000000"}, "password": {"waiterpass"}, "role": {"waiter"}})
	if body := c.get("/owner/staff"); !strings.Contains(body, "<code>waiterpass</code>") {
		t.Fatal("the Staff page does not show the staff password")
	}
	if body := c.get("/owner/staff"); strings.Contains(body, "firstpass1") {
		t.Fatal("the Staff page shows the owner's password")
	}
}

func TestUnconfirmedOwnerCantTakeGuestOrders(t *testing.T) {
	a, _, c := mailApp(t)
	// The demo restaurant's owner has no confirmed email. Verification off: guests can order.
	a.db.Exec(`UPDATE restaurants SET verify_email='required'`)
	if body := c.get("/t/demo-table-1"); notReady(body) {
		t.Fatal("blocked while verification is off")
	}
	a.cfg.EmailVerify = true
	if body := c.get("/t/demo-table-1"); !notReady(body) {
		t.Fatal("the table page doesn't say orders aren't taken yet")
	}
	c.do("POST", "/t/demo-table-1/enter", url.Values{"name": {"Rafi"}, "choice": {"new"}})
	if body := c.get("/t/demo-table-1"); !notReady(body) {
		t.Fatal("the menu doesn't say orders aren't taken yet")
	}
	c.post("/t/demo-table-1/cart/add", url.Values{"item_id": {itemID(t, a, "Borhani")}, "from": {"card"}})
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM cart_items`).Scan(&n)
	if n != 0 {
		t.Fatal("a guest could add to the cart before the owner confirmed their email")
	}
	a.db.Exec(`UPDATE staff_users SET email='demo@example.com', email_verified_at=1 WHERE role='owner'`)
	if body := c.get("/t/demo-table-1"); notReady(body) {
		t.Fatal("still blocked after the owner confirmed their email")
	}
}

// notReady reports whether a guest page says the restaurant isn't taking orders yet (in either language).
func notReady(body string) bool {
	return strings.Contains(body, "Not taking orders yet") || strings.Contains(body, "এখনো অর্ডার নেওয়া শুরু হয়নি")
}

func TestSealedPasswordsRoundTrip(t *testing.T) {
	a := &App{}
	var err error
	a.cfg.PasswordKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 bytes
	if a.pwKey, err = a.passwordCipher(); err != nil {
		t.Fatal(err)
	}
	s := a.sealPassword("waiterpass")
	if s == "waiterpass" || a.openPassword(s) != "waiterpass" {
		t.Fatal("seal/open does not round-trip")
	}
	if a.openPassword("") != "" || a.openPassword("garbage") != "" {
		t.Fatal("bad input should open to empty")
	}
}

func TestValidEmail(t *testing.T) {
	for in, want := range map[string]string{
		"a@b.co": "a@b.co", " a@b.co ": "a@b.co", "a@b": "", "nope": "", "A <a@b.co>": "", "": "",
	} {
		if got := validEmail(in); got != want {
			t.Errorf("validEmail(%q) = %q, want %q", in, got, want)
		}
	}
}
