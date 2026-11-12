// Package server holds the HTTP handlers: guest screens, staff screens and the owner dashboard.
package server

import (
	"bytes"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"time"
	_ "time/tzdata" // the container image has no time zone files

	"bosho/internal/config"
	"bosho/internal/database"
	"bosho/internal/storage"
	"bosho/web"
)

type App struct {
	cfg    config.Config
	db     *database.DB
	photos *storage.Photos
	tpl    *template.Template
	loc    *time.Location // restaurant-local time, used for "today" on the dashboard
}

func New(cfg config.Config, db *database.DB, photos *storage.Photos) (*App, error) {
	tpl, err := template.New("").Funcs(templateFuncs).ParseFS(web.Templates, "templates/*.html")
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		loc = time.FixedZone("BDT", 6*3600)
	}
	a := &App{cfg: cfg, db: db, photos: photos, tpl: tpl, loc: loc}
	if cfg.Demo {
		a.seedDemo()
	}
	return a, nil
}

// Handler returns the router with every route registered.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web.Static, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
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
	a.ownerRoutes(mux)
	return mux
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
