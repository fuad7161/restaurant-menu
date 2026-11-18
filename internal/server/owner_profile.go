package server

import (
	"net/http"
	"net/url"
	"strings"
)

func urlQuery(s string) string { return url.QueryEscape(s) }

// The restaurant profile: name, logo, brand colour and the details guests see.
// Only owners reach these handlers (see ownerRoutes).

func (a *App) profile(w http.ResponseWriter, r *http.Request, s *Staff) {
	q := r.URL.Query()
	a.render(w, "owner_profile", M{"Staff": s, "Page": "profile", "Swatches": Swatches,
		"Saved": q.Get("saved") != "", "Err": q.Get("err")})
}

func (a *App) profileSave(w http.ResponseWriter, r *http.Request, s *Staff) {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if err := r.ParseMultipartForm(6 << 20); err != nil {
		back(w, r, "/owner/profile?err="+urlQuery("The logo is too big. Use a file under 5 MB."))
		return
	}
	name := clean(r.FormValue("name"), 60)
	if name == "" {
		name = s.R.Name
	}
	color := strings.ToLower(strings.TrimSpace(r.FormValue("color")))
	if color == "custom" {
		color = strings.ToLower(strings.TrimSpace(r.FormValue("custom_color")))
	}
	errMsg := ""
	if !usableColor(color) {
		errMsg = "That colour is too light: text on it would be hard to read. Pick a darker one."
		color = s.R.Color
	}
	a.db.Exec(`UPDATE restaurants SET name=?,brand_color=?,tagline=?,address=?,phone=? WHERE id=?`,
		name, color, clean(r.FormValue("tagline"), 80), clean(r.FormValue("address"), 120),
		clean(r.FormValue("phone"), 20), s.R.ID)
	if _, _, err := r.FormFile("logo"); err == nil {
		if logo := a.savePhoto(r, "logo"); logo != "" {
			a.db.Exec(`UPDATE restaurants SET logo=? WHERE id=?`, logo, s.R.ID)
		} else {
			errMsg = "The logo could not be saved. Use a JPEG, PNG or WebP image."
		}
	}
	if errMsg != "" {
		http.Redirect(w, r, "/owner/profile?err="+urlQuery(errMsg), http.StatusSeeOther)
		return
	}
	back(w, r, "/owner/profile?saved=1")
}

func (a *App) logoDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.db.Exec(`UPDATE restaurants SET logo='' WHERE id=?`, s.R.ID)
	back(w, r, "/owner/profile?saved=1")
}
