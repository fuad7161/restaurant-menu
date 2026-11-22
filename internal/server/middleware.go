package server

import (
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// handlerTimeout is the longest a request may take before the guest or staff member gets
// "try again" instead of a screen that hangs. Photo uploads are exempt (slow mobile data).
const handlerTimeout = 20 * time.Second

// protect wraps every route with the safety layers, outermost first:
// request id and logging, crash recovery, security headers, the cross-site check, the guest
// rate limit and the time limit.
func (a *App) protect(next http.Handler) http.Handler {
	timed := http.TimeoutHandler(next, handlerTimeout, "The server is busy. Please try again.")
	return a.logRequests(recoverPanics(securityHeaders(sameOrigin(a.limitGuests(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
				next.ServeHTTP(w, r)
				return
			}
			timed.ServeHTTP(w, r)
		}))))))
}

// statusWriter remembers the status code for the request log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// logRequests gives every request an id (sent back as X-Request-Id) and logs the ones worth
// looking at: server errors, refused requests and slow ones. The screens that refresh every few
// seconds would drown everything else, so ordinary successes are not logged.
func (a *App) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := randToken(6)
		w.Header().Set("X-Request-Id", id)
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r)
		took := time.Since(start)
		if sw.status >= 500 || sw.status == http.StatusTooManyRequests || sw.status == http.StatusForbidden || took > 2*time.Second {
			ip, _ := a.clientIP(r)
			slog.Warn("request", "id", id, "method", r.Method, "path", r.URL.Path, "status", sw.status,
				"ms", took.Milliseconds(), "ip", ip.String())
		}
	})
}

// recoverPanics turns a crash inside one request into a logged error page, so one bad request
// can never take the kitchen screen down for everyone.
func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic", "path", r.URL.Path, "error", v, "stack", string(debug.Stack()))
				http.Error(w, "Something went wrong. Please try again.", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders: no framing by other sites, no content sniffing, no full URLs (with table
// tokens) leaking to other sites, and a content policy that allows only this site's own files.
// Inline styles and handlers are allowed because the templates use a few.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "+
			"script-src 'self' 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// sameOrigin refuses form posts and taps that come from another website (cross-site request
// forgery). Browsers say where a request comes from in Sec-Fetch-Site, and older ones in Origin.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			site := r.Header.Get("Sec-Fetch-Site")
			bad := site == "cross-site" || site == "same-site"
			if o := r.Header.Get("Origin"); !bad && site == "" && o != "" && o != "null" {
				u, err := url.Parse(o)
				bad = err != nil || u.Host != r.Host
			}
			if bad {
				http.Error(w, "This request came from another website and was refused.", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// limiter counts events per key in fixed time windows. It lives in memory: with one machine
// that is all the app needs, and a restart simply starts the counts again.
type limiter struct {
	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	start time.Time
	count int
}

func newLimiter() *limiter { return &limiter{windows: map[string]*window{}} }

// hit counts one event for key and reports whether it is still within max per period.
func (l *limiter) hit(key string, max int, period time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.windows[key]
	if w == nil || time.Since(w.start) > period {
		w = &window{start: time.Now()}
		l.windows[key] = w
	}
	w.count++
	return w.count <= max
}

// count reports how many events key has in its current window, without adding one.
func (l *limiter) count(key string, period time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if w := l.windows[key]; w != nil && time.Since(w.start) <= period {
		return w.count
	}
	return 0
}

func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.windows, key)
	l.mu.Unlock()
}

// sweep forgets windows older than maxAge (run by the maintenance job).
func (l *limiter) sweep(maxAge time.Duration) {
	l.mu.Lock()
	for k, w := range l.windows {
		if time.Since(w.start) > maxAge {
			delete(l.windows, k)
		}
	}
	l.mu.Unlock()
}

// Guest limits: a phone may send at most guestPosts taps a minute (adding to the cart, changing
// it, calling a waiter...), and one network at most networkPosts, so a script cannot flood a
// restaurant with orders or calls.
const (
	guestPosts   = 60
	networkPosts = 600
)

func (a *App) limitGuests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/t/") {
			dev := ""
			if c, err := r.Cookie("bosho_dev"); err == nil {
				dev = c.Value
			}
			ip, _ := a.clientIP(r)
			if (dev != "" && !a.limits.hit("guest:"+dev, guestPosts, time.Minute)) ||
				!a.limits.hit("guestnet:"+ip.String(), networkPosts, time.Minute) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "Too many requests. Please wait a minute.", http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// secure reports whether the browser reached us over HTTPS (directly or through the proxy),
// so cookies can be marked Secure there and still work on plain http on the local Wi-Fi.
func secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// setCookie sets a cookie that is HttpOnly unless told otherwise, SameSite=Lax, and Secure on HTTPS.
func setCookie(w http.ResponseWriter, r *http.Request, c *http.Cookie) {
	c.Path = "/"
	c.SameSite = http.SameSiteLaxMode
	c.Secure = secure(r)
	http.SetCookie(w, c)
}
