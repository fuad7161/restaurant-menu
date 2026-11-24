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
	// The Google review link must be a web address; anything else is dropped.
	review := strings.TrimSpace(r.FormValue("review_url"))
	if u, err := url.Parse(review); err != nil || u.Scheme != "https" || u.Host == "" || len(review) > 300 {
		if review != "" && errMsg == "" {
			errMsg = "The review link must start with https://"
		}
		review = ""
	}
	if err := exec(a.db, `UPDATE restaurants SET name=?,brand_color=?,tagline=?,address=?,phone=?,review_url=?,bin=? WHERE id=?`,
		name, color, clean(r.FormValue("tagline"), 80), clean(r.FormValue("address"), 120),
		clean(r.FormValue("phone"), 20), review, clean(r.FormValue("bin"), 30), s.R.ID); err != nil {
		serverError(w, err)
		return
	}
	if _, _, err := r.FormFile("logo"); err == nil {
		if logo := a.savePhoto(r, "logo"); logo != "" {
			a.db.Exec(`UPDATE restaurants SET logo=? WHERE id=?`, logo, s.R.ID)
		} else {
			errMsg = "The logo could not be saved. Use a JPEG, PNG or WebP image."
		}
	}
	a.syncOneBranchBusiness(s.R.ID)
	if errMsg != "" {
		http.Redirect(w, r, "/owner/profile?err="+urlQuery(errMsg), http.StatusSeeOther)
		return
	}
	back(w, r, "/owner/profile?saved=1")
}

func (a *App) logoDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.db.Exec(`UPDATE restaurants SET logo='' WHERE id=?`, s.R.ID)
	a.syncOneBranchBusiness(s.R.ID)
	back(w, r, "/owner/profile?saved=1")
}

// syncOneBranchBusiness copies a branch's name, logo, colour and tagline to its business when the
// business has only this branch, so the business stays named and branded like the restaurant the
// owner edits on the Profile page. A business with several branches has its own profile.
func (a *App) syncOneBranchBusiness(branchID int64) {
	a.db.Exec(`UPDATE businesses b SET name=r.name,logo=r.logo,brand_color=r.brand_color,tagline=r.tagline
		FROM restaurants r WHERE r.id=? AND b.id=r.business_id
		AND (SELECT COUNT(*) FROM restaurants x WHERE x.business_id=b.id)=1`, branchID)
}
