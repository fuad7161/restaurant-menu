package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeVersions struct {
	n    atomic.Int64
	down atomic.Bool
}

func (f *fakeVersions) Get(context.Context) (int64, bool) { return f.n.Load(), !f.down.Load() }
func (f *fakeVersions) Bump(context.Context)              { f.n.Add(1) }

// pollApp is an App with only what poll and renderPoll need, and one refreshing endpoint
// that counts how often it really ran (each run stands for the database queries).
func pollApp(t *testing.T) (*App, *fakeVersions, http.Handler, *int) {
	t.Helper()
	fixed := time.Unix(1_800_000_000, 0)
	pollClock = func() time.Time { return fixed }
	t.Cleanup(func() { pollClock = time.Now })
	tpl, err := newTemplates()
	if err != nil {
		t.Fatal(err)
	}
	fv := &fakeVersions{}
	a := &App{tpl: tpl, ver: fv}
	runs, body := 0, "board v1"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /board", a.poll(func(w http.ResponseWriter, r *http.Request) {
		runs++
		a.renderPoll(w, r, "g_msg", body)
	}))
	mux.HandleFunc("POST /change", func(w http.ResponseWriter, r *http.Request) { body = "board v2" })
	return a, fv, a.bumpOnWrite(mux), &runs
}

func get(h http.Handler, seen, cookie string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/board", nil)
	if seen != "" {
		r.Header.Set("X-Seen", seen)
	}
	if cookie != "" {
		r.Header.Set("Cookie", cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPollSkipsWorkWhenNothingChanged(t *testing.T) {
	_, _, h, runs := pollApp(t)
	first := get(h, "", "bosho_staff=a")
	if first.Code != 200 || *runs != 1 {
		t.Fatalf("first refresh: code %d runs %d", first.Code, *runs)
	}
	tok := first.Header().Get("X-Hash")
	again := get(h, tok, "bosho_staff=a")
	if again.Code != 204 || *runs != 1 {
		t.Fatalf("unchanged refresh should be 204 without running the handler: code %d runs %d", again.Code, *runs)
	}
}

func TestPollRebuildsAfterAWrite(t *testing.T) {
	_, fv, h, runs := pollApp(t)
	tok := get(h, "", "c=1").Header().Get("X-Hash")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/change", nil))
	if fv.n.Load() != 1 {
		t.Fatalf("POST should bump the counter, got %d", fv.n.Load())
	}
	after := get(h, tok, "c=1")
	if after.Code != 200 || *runs != 2 || after.Body.String() != "<p class=\"warn\">board v2</p>" {
		t.Fatalf("refresh after a write must show new data: code %d runs %d body %q", after.Code, *runs, after.Body.String())
	}
}

func TestPollGetDoesNotBump(t *testing.T) {
	_, fv, h, _ := pollApp(t)
	get(h, "", "")
	if fv.n.Load() != 0 {
		t.Fatal("a GET must not bump the counter")
	}
}

func TestPollTokenIsPerBrowser(t *testing.T) {
	_, _, h, runs := pollApp(t)
	tok := get(h, "", "bosho_dev=guest1").Header().Get("X-Hash")
	other := get(h, tok, "bosho_dev=guest2; lang=en")
	if *runs != 2 {
		t.Fatalf("another browser's token must not skip the work (code %d)", other.Code)
	}
}

func TestPollTokenExpiresWithTheTimeSlot(t *testing.T) {
	_, _, h, runs := pollApp(t)
	tok := get(h, "", "c=1").Header().Get("X-Hash")
	later := time.Unix(1_800_000_000, 0).Add(pollSlot)
	pollClock = func() time.Time { return later }
	get(h, tok, "c=1")
	if *runs != 2 {
		t.Fatal("a token from an earlier time slot must rebuild (\"2 min ago\" texts move on)")
	}
}

func TestPollWithoutRedisStillWorks(t *testing.T) {
	_, fv, h, runs := pollApp(t)
	fv.down.Store(true)
	tok := get(h, "", "c=1").Header().Get("X-Hash")
	again := get(h, tok, "c=1")
	if *runs != 2 {
		t.Fatal("without Redis every refresh must run the handler")
	}
	if again.Code != 204 {
		t.Fatalf("unchanged HTML should still be 204 (no repaint), got %d", again.Code)
	}
}
