package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func urlQuery(s string) string { return url.QueryEscape(s) }

// Profiles (docs/plans/multi-branch.md, 8.3). The business has the name, logo, colour and tagline
// shared by its branches. A branch has its address, phone, BIN and Google review link, and may
// override the logo, colour and tagline (empty = the business's).
//
//   - /owner/profile works on the chosen branch. An owner with one branch edits the business's
//     branding here too, so the page looks as it always has. An owner with several branches can
//     give this branch its own branding or use the business's. A manager edits only the details.
//   - /owner/business is the business's branding, for owners with several branches.

// profileForm is what the profile template shows and where it posts.
type profileForm struct {
	Action       string     // where the form posts; the logo is removed at Action+"/logo/delete"
	P            Restaurant // the values shown: Name, Logo, Color, Tagline, and the branch details
	Brand        bool       // show logo, colour and tagline
	Name         bool       // show the name field (the business's name)
	Details      bool       // show address, phone, BIN and review link
	Override     bool       // show "use the business's logo, colour and tagline"
	UsesBusiness bool       // this branch has no branding of its own
}

func (a *App) profile(w http.ResponseWriter, r *http.Request, s *Staff) {
	brand := s.Can(PermBusiness)
	f := profileForm{Action: "/owner/profile", P: s.R, Brand: brand, Name: brand && !s.MultiBranch(), Details: true, Override: brand && s.MultiBranch()}
	var own string
	a.db.QueryRow(`SELECT logo||brand_color||tagline FROM restaurants WHERE id=?`, s.R.ID).Scan(&own)
	f.UsesBusiness = own == ""
	a.renderProfile(w, r, s, f)
}

func (a *App) business(w http.ResponseWriter, r *http.Request, s *Staff) {
	f := profileForm{Action: "/owner/business", Brand: true, Name: true}
	a.db.QueryRow(`SELECT name,logo,brand_color,tagline FROM businesses WHERE id=?`, s.R.BusinessID).Scan(&f.P.Name, &f.P.Logo, &f.P.Color, &f.P.Tagline)
	a.renderProfile(w, r, s, f)
}

func (a *App) renderProfile(w http.ResponseWriter, r *http.Request, s *Staff, f profileForm) {
	q := r.URL.Query()
	page := "profile"
	if f.Action == "/owner/business" {
		page = "business"
	}
	a.render(w, "owner_profile", M{"Staff": s, "Page": page, "F": f, "Swatches": Swatches, "Saved": q.Get("saved") != "", "Err": q.Get("err")})
}

func (a *App) profileSave(w http.ResponseWriter, r *http.Request, s *Staff) {
	if !a.parseProfile(w, r, "/owner/profile") {
		return
	}
	errMsg := ""
	// The details of this branch.
	review := strings.TrimSpace(r.FormValue("review_url"))
	if u, err := url.Parse(review); err != nil || u.Scheme != "https" || u.Host == "" || len(review) > 300 {
		if review != "" {
			errMsg = "The review link must start with https://"
		}
		review = ""
	}
	if err := exec(a.db, `UPDATE restaurants SET address=?,phone=?,review_url=?,bin=? WHERE id=?`,
		clean(r.FormValue("address"), 120), clean(r.FormValue("phone"), 20), review, clean(r.FormValue("bin"), 30), s.R.ID); err != nil {
		serverError(w, err)
		return
	}
	switch {
	case !s.Can(PermBusiness): // a manager: details only
	case !s.MultiBranch():
		// One branch: the branding is the business's, and the branch keeps none of its own.
		if e := a.saveBrand(r, "businesses", s.R.BusinessID, s.R.Color); e != "" {
			errMsg = e
		}
		a.db.Exec(`UPDATE restaurants SET logo='',brand_color='',tagline='' WHERE business_id=?`, s.R.BusinessID)
	case r.FormValue("use_business") != "":
		a.db.Exec(`UPDATE restaurants SET logo='',brand_color='',tagline='' WHERE id=?`, s.R.ID)
	default:
		if e := a.saveBrand(r, "restaurants", s.R.ID, s.R.Color); e != "" {
			errMsg = e
		}
	}
	a.profileDone(w, r, "/owner/profile", errMsg)
}

func (a *App) businessSave(w http.ResponseWriter, r *http.Request, s *Staff) {
	if !a.parseProfile(w, r, "/owner/business") {
		return
	}
	var cur string
	a.db.QueryRow(`SELECT brand_color FROM businesses WHERE id=?`, s.R.BusinessID).Scan(&cur)
	a.profileDone(w, r, "/owner/business", a.saveBrand(r, "businesses", s.R.BusinessID, cur))
}

func (a *App) parseProfile(w http.ResponseWriter, r *http.Request, page string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	// A form without a file to upload may come as a plain form.
	if err := r.ParseMultipartForm(6 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		back(w, r, page+"?err="+urlQuery("The logo is too big. Use a file under 5 MB."))
		return false
	}
	return true
}

func (a *App) profileDone(w http.ResponseWriter, r *http.Request, page, errMsg string) {
	if errMsg != "" {
		http.Redirect(w, r, page+"?err="+urlQuery(errMsg), http.StatusSeeOther)
		return
	}
	back(w, r, page+"?saved=1")
}

// saveBrand saves the name (when the form has one), colour, tagline and an uploaded logo into the
// businesses or restaurants row id. A business's name is also kept on its branches' rows, which
// lists use for a branch without a name of its own. It returns a message for the owner, or "".
func (a *App) saveBrand(r *http.Request, table string, id int64, curColor string) string {
	if table != "businesses" && table != "restaurants" {
		panic("saveBrand: " + table)
	}
	errMsg := ""
	color := strings.ToLower(strings.TrimSpace(r.FormValue("color")))
	if color == "custom" {
		color = strings.ToLower(strings.TrimSpace(r.FormValue("custom_color")))
	}
	if !usableColor(color) {
		errMsg = "That colour is too light: text on it would be hard to read. Pick a darker one."
		color = curColor
	}
	a.db.Exec(`UPDATE `+table+` SET brand_color=?,tagline=? WHERE id=?`, color, clean(r.FormValue("tagline"), 80), id)
	if name := clean(r.FormValue("name"), 60); name != "" && table == "businesses" {
		a.db.Exec(`UPDATE businesses SET name=? WHERE id=?`, name, id)
		a.db.Exec(`UPDATE restaurants SET name=? WHERE business_id=?`, name, id)
	}
	if _, _, err := r.FormFile("logo"); err == nil {
		if logo := a.savePhoto(r, "logo"); logo != "" {
			a.db.Exec(`UPDATE `+table+` SET logo=? WHERE id=?`, logo, id)
		} else {
			errMsg = "The logo could not be saved. Use a JPEG, PNG or WebP image."
		}
	}
	return errMsg
}

// logoDelete removes the logo shown on this branch: the business's with one branch, else the
// branch's own (the branch then shows the business's logo).
func (a *App) logoDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	if !s.Can(PermBusiness) {
		refuse(w, r, s, PermBusiness)
		return
	}
	if s.MultiBranch() {
		a.db.Exec(`UPDATE restaurants SET logo='' WHERE id=?`, s.R.ID)
	} else {
		a.db.Exec(`UPDATE businesses SET logo='' WHERE id=?`, s.R.BusinessID)
		a.db.Exec(`UPDATE restaurants SET logo='' WHERE business_id=?`, s.R.BusinessID)
	}
	back(w, r, "/owner/profile?saved=1")
}

func (a *App) businessLogoDelete(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.db.Exec(`UPDATE businesses SET logo='' WHERE id=?`, s.R.BusinessID)
	back(w, r, "/owner/business?saved=1")
}
