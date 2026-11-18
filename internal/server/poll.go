package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Screens that refresh themselves (kitchen board, counter list, guest bar, cart, orders, waiting
// screen) send back the token from their last reply in X-Seen. A token is
//
//	<change counter>.<time slot>.<who>.<content hash>
//
// - change counter: the Redis counter read before the reply was built ("-" without Redis).
// - time slot: replies hold texts like "2 min ago", so a token only lasts one slot.
// - who: a hash of the path, query and cookies, so a token only works for the browser,
//   language and screen it was made for.
// - content hash: a hash of the HTML itself.
//
// If the counter and slot are unchanged, nothing was written since the reply was built, and
// the server answers 204 at once, without PostgreSQL. Otherwise the fragment is built as
// usual, and is still answered with 204 if its HTML came out the same.

// pollSlot is how long a token can skip the database. It is also the most a lost Redis
// update could delay a screen.
const pollSlot = 15 * time.Second

// versionStore is the shared change counter (cache.Versions; a fake in tests).
type versionStore interface {
	Get(ctx context.Context) (int64, bool)
	Bump(ctx context.Context)
}

type pollKey struct{}

type pollState struct{ version, slot, who string }

// pollClock is replaced in tests.
var pollClock = time.Now

func pollSlotNow() string { return strconv.FormatInt(pollClock().UnixNano()/int64(pollSlot), 36) }

func pollWho(r *http.Request) string {
	h := sha256.Sum256([]byte(r.URL.Path + "?" + r.URL.RawQuery + "\n" + r.Header.Get("Cookie")))
	return hex.EncodeToString(h[:6])
}

// poll wraps a refresh endpoint with the Redis shortcut.
func (a *App) poll(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st := pollState{version: "-", slot: pollSlotNow(), who: pollWho(r)}
		// Read the counter before any data is loaded: data read later can only be newer.
		if n, ok := a.ver.Get(r.Context()); ok {
			st.version = strconv.FormatInt(n, 36)
			if seen := strings.Split(r.Header.Get("X-Seen"), "."); len(seen) == 4 &&
				seen[0] == st.version && seen[1] == st.slot && seen[2] == st.who {
				w.Header().Set("X-Hash", r.Header.Get("X-Seen"))
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		h(w, r.WithContext(context.WithValue(r.Context(), pollKey{}, st)))
	}
}

// bumpOnWrite bumps the change counter after every request that can change data (anything but
// GET and HEAD). It runs after the handler, so the change is already saved in PostgreSQL.
func (a *App) bumpOnWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			a.changed()
		}
	})
}

// changed tells every refreshing screen to rebuild on its next refresh.
func (a *App) changed() {
	a.ver.Bump(context.Background())
}
