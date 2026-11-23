// Package server holds the HTTP handlers: guest screens, staff screens and the owner dashboard.
package server

import (
	"bytes"
	"crypto/cipher"
	"fmt"
	"hash/fnv"
	"html/template"
	"io/fs"
	"log"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // the container image has no time zone files

	"bosho/internal/cache"
	"bosho/internal/config"
	"bosho/internal/database"
	"bosho/internal/mail"
	"bosho/internal/storage"
	"bosho/web"
)

type App struct {
	cfg    config.Config
	db     *database.DB
	photos *storage.Photos
	tpl    *template.Template
	ver    versionStore   // Redis change counter; nothing configured = no shortcut
	loc    *time.Location // restaurant-local time, used for "today" on the dashboard
	limits *limiter       // login attempts and guest taps (see middleware.go)
	mail   mail.Sender    // verify-email and reset-password emails (log only without RESEND_API_KEY)
	pwKey  cipher.AEAD    // encrypts staff passwords the owner can look up (see secret.go)
}

// New builds the app. ver may be nil (no Redis): refreshes then always query PostgreSQL.
func New(cfg config.Config, db *database.DB, photos *storage.Photos, ver *cache.Versions) (*App, error) {
	tpl, err := newTemplates()
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		loc = time.FixedZone("BDT", 6*3600)
	}
	a := &App{cfg: cfg, db: db, photos: photos, tpl: tpl, loc: loc, ver: ver, limits: newLimiter(),
		mail: mail.New(mail.Settings{BrevoKey: cfg.BrevoAPIKey, SMTPHost: cfg.SMTPHost, SMTPPort: cfg.SMTPPort, SMTPUser: cfg.SMTPUser,
			SMTPPassword: cfg.SMTPPassword, ResendKey: cfg.ResendAPIKey, From: cfg.EmailFrom})}
	slog.Info("email sender", "via", strings.TrimPrefix(fmt.Sprintf("%T", a.mail), "*mail."))
	if a.pwKey, err = a.passwordCipher(); err != nil {
		return nil, err
	}
	if cfg.Demo {
		a.seedDemo()
	}
	return a, nil
}

func init() {
	// The staff screens' web app manifest (Go does not know this extension).
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

func newTemplates() (*template.Template, error) {
	return template.New("").Funcs(templateFuncs).ParseFS(web.Templates, "templates/*.html")
}

// Handler returns the router with every route registered.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web.Static, "static")
	files := http.StripPrefix("/static/", http.FileServerFS(static))
	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		// Versioned links (see asset) and font files never change: cache them for a year.
		// Anything else, such as images named in the CSS, for a day.
		if r.URL.Query().Get("v") != "" || strings.HasPrefix(r.URL.Path, "/static/fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		files.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /uploads/{name}", func(w http.ResponseWriter, r *http.Request) {
		a.photos.Serve(w, r, r.PathValue("name"))
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		var one int
		if err := a.db.QueryRow(`SELECT 1`).Scan(&one); err != nil {
			http.Error(w, "database down", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	a.guestRoutes(mux)
	a.staffRoutes(mux)
	a.emailRoutes(mux)
	a.ownerRoutes(mux)
	return a.protect(a.bumpOnWrite(mux))
}

func (a *App) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := a.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Println("template", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

// renderPoll renders a fragment that the page refreshes every few seconds. The response carries
// a token in X-Hash (see poll.go); app.js sends it back as X-Seen on the next poll. When the HTML
// is unchanged the reply is 204 No Content, which htmx does not swap: no repaint, no flicker, and
// only a few bytes on the wire instead of the whole board.
func (a *App) renderPoll(w http.ResponseWriter, r *http.Request, name string, data any) {
	var buf bytes.Buffer
	if err := a.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Println("template", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	h := fnv.New64a()
	h.Write(buf.Bytes())
	sum := strconv.FormatUint(h.Sum64(), 36)
	st, ok := r.Context().Value(pollKey{}).(pollState)
	if !ok {
		st = pollState{version: "-", slot: pollSlotNow(), who: pollWho(r)}
	}
	w.Header().Set("X-Hash", st.version+"."+st.slot+"."+st.who+"."+sum)
	w.Header().Set("Cache-Control", "no-store")
	if seen := strings.Split(r.Header.Get("X-Seen"), "."); len(seen) == 4 && seen[3] == sum {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

// base is the public address of the site, used inside QR codes.
func (a *App) base(r *http.Request) string {
	if a.cfg.BaseURL != "" {
		return a.cfg.BaseURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
