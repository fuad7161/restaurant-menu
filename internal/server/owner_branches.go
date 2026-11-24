package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bosho/internal/database"
)

// Branches (docs/plans/multi-branch.md): a business has one or more branches, each a row in
// restaurants. The owner adds, edits, closes and archives them here, and switches between them.

func (a *App) branchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /owner/branches", a.need(PermBranches, a.ownerBranches))
	mux.HandleFunc("POST /owner/branches", a.need(PermBranches, a.branchCreate))
	mux.HandleFunc("POST /owner/branches/{id}", a.need(PermBranches, a.branchUpdate))
	mux.HandleFunc("POST /owner/branches/{id}/status", a.need(PermBranches, a.branchStatus))
	mux.HandleFunc("POST /owner/branch", a.need(PermBranches, a.branchSwitch))
}

// branchesOf lists the branches, not archived, of the business that branch rid belongs to.
func (a *App) branchesOf(rid int64) []Branch {
	return database.List(a.db, `SELECT id,CASE WHEN branch_name!='' THEN branch_name ELSE name END,branch_code,status
		FROM restaurants WHERE business_id=(SELECT business_id FROM restaurants WHERE id=?) AND status!='archived'
		ORDER BY sort_order,id`,
		func(r *sql.Rows, b *Branch) error { return r.Scan(&b.ID, &b.Label, &b.Code, &b.Status) }, rid)
}

// pickBranch answers a page that works on one branch while the owner is in "All branches" mode:
// it shows the branches to choose from, and choosing one comes back to the same page.
func (a *App) pickBranch(w http.ResponseWriter, r *http.Request, s *Staff) {
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", pageOf(r.Header.Get("HX-Current-URL")))
		return
	}
	if r.Method != http.MethodGet {
		back(w, r, "/owner/branches")
		return
	}
	a.render(w, "pick_branch", M{"Staff": s, "Page": "pick", "Next": r.URL.RequestURI()})
}

// branchSwitch saves the branch the owner works on for this login: a branch id, or "all".
func (a *App) branchSwitch(w http.ResponseWriter, r *http.Request, s *Staff) {
	var to int64
	if v := r.FormValue("branch"); v != "all" || !s.MultiBranch() {
		fmt.Sscan(v, &to)
		found := false
		for _, b := range s.Branches {
			found = found || b.ID == to
		}
		if !found {
			http.NotFound(w, r)
			return
		}
	}
	c, _ := r.Cookie("bosho_staff")
	if err := exec(a.db, `UPDATE staff_sessions SET branch_id=? WHERE token=? AND staff_id=?`, to, c.Value, s.ID); err != nil {
		serverError(w, err)
		return
	}
	next := safeNext(r.FormValue("next"))
	if next == "" {
		next = safeNext(pageOf(r.Referer())) // back to the page the switcher was used on
	}
	if next == "" || strings.HasPrefix(next, "/owner/branch?") {
		next = "/owner"
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", next)
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

type branchRow struct {
	ID                               int64
	BranchName, Code, Address, Phone string
	Status                           string
	Sales                            int64
	Tables, People                   int
}

func (a *App) ownerBranches(w http.ResponseWriter, r *http.Request, s *Staff) {
	var max int
	a.db.QueryRow(`SELECT max_branches FROM businesses WHERE id=?`, s.R.BusinessID).Scan(&max)
	rows := database.List(a.db, `SELECT r.id,r.branch_name,r.branch_code,r.address,r.phone,r.status,
		(SELECT COALESCE(SUM(p.amount),0)::BIGINT FROM payments p WHERE p.restaurant_id=r.id AND p.created_at>=?),
		(SELECT COUNT(*) FROM tables t WHERE t.restaurant_id=r.id),
		(SELECT COUNT(*) FROM staff_users u WHERE u.restaurant_id=r.id AND u.role!='owner')
		FROM restaurants r WHERE r.business_id=? ORDER BY r.status='archived',r.sort_order,r.id`,
		func(rs *sql.Rows, b *branchRow) error {
			return rs.Scan(&b.ID, &b.BranchName, &b.Code, &b.Address, &b.Phone, &b.Status, &b.Sales, &b.Tables, &b.People)
		}, a.dayStart(), s.R.BusinessID)
	q := r.URL.Query()
	a.render(w, "owner_branches", M{"Staff": s, "Page": "branches", "Rows": rows, "Max": max, "InUse": len(s.Branches),
		"Err": q.Get("err"), "Done": q.Get("done")})
}

// dayStart is midnight today in the restaurant's time zone, as Unix seconds.
func (a *App) dayStart() int64 {
	t := time.Now().In(a.loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, a.loc).Unix()
}

// branchCode is a short code for invoices: letters and digits, upper case, at most 6.
func branchCode(in string) (string, bool) {
	in = strings.ToUpper(strings.TrimSpace(in))
	if len(in) > 6 {
		return "", false
	}
	for _, c := range in {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return "", false
		}
	}
	return in, true
}

func branchErr(w http.ResponseWriter, r *http.Request, msg string) {
	back(w, r, "/owner/branches?err="+urlQuery(msg))
}

// codeTaken reports whether another branch of the business already prints this invoice code.
func (a *App) codeTaken(bid, except int64, code string) bool {
	if code == "" {
		return false
	}
	var taken bool
	a.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM restaurants WHERE business_id=? AND id!=? AND branch_code=?)`, bid, except, code).Scan(&taken)
	return taken
}

// atBranchLimit reports whether the business already has as many branches in use (not archived)
// as the platform admin allows.
func (a *App) atBranchLimit(bid int64) (bool, int) {
	var inUse, max int
	a.db.QueryRow(`SELECT (SELECT COUNT(*) FROM restaurants WHERE business_id=b.id AND status!='archived'),b.max_branches
		FROM businesses b WHERE b.id=?`, bid).Scan(&inUse, &max)
	return inUse >= max, max
}

// branchCreate adds a branch. With a source branch chosen it copies, as ticked: the menu
// (categories, dishes, photos, option groups and options), the settings (charges, hours, ticket
// colours, accept rule) and the logo, colour and tagline. Tables and staff are never copied.
func (a *App) branchCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	name := clean(r.FormValue("branch_name"), 40)
	code, ok := branchCode(r.FormValue("code"))
	switch {
	case name == "":
		branchErr(w, r, "Give the branch a name, for example the area it is in.")
		return
	case !ok:
		branchErr(w, r, "The code can have up to 6 letters and digits, for example GUL.")
		return
	case a.codeTaken(s.R.BusinessID, 0, code):
		branchErr(w, r, "Another branch already uses the code "+code+".")
		return
	}
	if full, max := a.atBranchLimit(s.R.BusinessID); full {
		branchErr(w, r, fmt.Sprintf("Your plan allows %d branches. Ask Bosho to raise the limit, or archive a branch you no longer use.", max))
		return
	}
	var from int64
	fmt.Sscan(r.FormValue("from"), &from)
	if from != 0 {
		var mine bool
		a.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM restaurants WHERE id=? AND business_id=?)`, from, s.R.BusinessID).Scan(&mine)
		if !mine {
			http.NotFound(w, r)
			return
		}
	}
	copyMenu, copySettings, copyBrand := from != 0 && r.FormValue("copy_menu") != "", from != 0 && r.FormValue("copy_settings") != "", from != 0 && r.FormValue("copy_brand") != ""

	var bizName string
	a.db.QueryRow(`SELECT name FROM businesses WHERE id=?`, s.R.BusinessID).Scan(&bizName)
	var rid int64
	err := a.inTx(func(q database.Querier) error {
		var err error
		// The email verification setting is the platform admin's, per business: the new branch
		// follows the one the owner is working in.
		rid, err = database.InsertID(q, `INSERT INTO restaurants(business_id,name,branch_name,branch_code,address,phone,slug,verify_email,sort_order,created_at)
			VALUES(?,?,?,?,?,?,?,?,(SELECT COALESCE(MAX(sort_order),0)+1 FROM restaurants WHERE business_id=?),?)`,
			s.R.BusinessID, bizName, name, code, clean(r.FormValue("address"), 120), clean(r.FormValue("phone"), 20), randToken(6), s.R.VerifyEmail, s.R.BusinessID, now())
		if err != nil {
			return err
		}
		if copySettings {
			if err := exec(q, `UPDATE restaurants t SET vat_percent=f.vat_percent,service_percent=f.service_percent,require_accept=f.require_accept,
				open_time=f.open_time,close_time=f.close_time,last_order=f.last_order,warn_minutes=f.warn_minutes,late_minutes=f.late_minutes,
				orders_in_hours=f.orders_in_hours FROM restaurants f WHERE f.id=? AND t.id=?`, from, rid); err != nil {
				return err
			}
		}
		if copyBrand {
			if err := exec(q, `UPDATE restaurants t SET logo=f.logo,brand_color=f.brand_color,tagline=f.tagline FROM restaurants f WHERE f.id=? AND t.id=?`, from, rid); err != nil {
				return err
			}
		} // else the new branch uses the business's logo, colour and tagline (its own stay empty)
		if copyMenu {
			return copyMenuTo(q, from, rid)
		}
		return nil
	})
	if err != nil {
		serverError(w, err)
		return
	}
	a.audit(s.R.ID, s.Name, "branch added", name)
	back(w, r, "/owner/branches?done=added")
}

// copyMenuTo copies the menu of branch from into branch to, with new ids: categories, then their
// dishes, then each dish's option groups, then its options in their new groups. Photos are only
// file names, so both branches share the same files. Deleted dishes are not copied.
func copyMenuTo(q database.Querier, from, to int64) error {
	type pair struct{ old, new int64 }
	cats := map[int64]int64{}
	var catRows []pair
	rows, err := q.Query(`SELECT id FROM menu_categories WHERE restaurant_id=? ORDER BY id`, from)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p pair
		rows.Scan(&p.old)
		catRows = append(catRows, p)
	}
	rows.Close()
	for _, c := range catRows {
		id, err := database.InsertID(q, `INSERT INTO menu_categories(restaurant_id,name_bn,name_en,sort_order)
			SELECT ?,name_bn,name_en,sort_order FROM menu_categories WHERE id=?`, to, c.old)
		if err != nil {
			return err
		}
		cats[c.old] = id
	}

	var items []pair
	rows, err = q.Query(`SELECT id,category_id FROM menu_items WHERE restaurant_id=? AND NOT deleted ORDER BY id`, from)
	if err != nil {
		return err
	}
	var itemCats []int64
	for rows.Next() {
		var p pair
		var cat int64
		rows.Scan(&p.old, &cat)
		items = append(items, p)
		itemCats = append(itemCats, cat)
	}
	rows.Close()
	for i, it := range items {
		cat, ok := cats[itemCats[i]]
		if !ok {
			return errors.New("dish in a category of another branch")
		}
		id, err := database.InsertID(q, `INSERT INTO menu_items(restaurant_id,category_id,name_bn,name_en,desc_bn,desc_en,price,photo,prep_minutes,is_available,tags,featured)
			SELECT ?,?,name_bn,name_en,desc_bn,desc_en,price,photo,prep_minutes,is_available,tags,featured FROM menu_items WHERE id=?`, to, cat, it.old)
		if err != nil {
			return err
		}
		items[i].new = id

		groups := map[int64]int64{}
		var gs []int64
		grows, err := q.Query(`SELECT id FROM option_groups WHERE menu_item_id=? ORDER BY id`, it.old)
		if err != nil {
			return err
		}
		for grows.Next() {
			var g int64
			grows.Scan(&g)
			gs = append(gs, g)
		}
		grows.Close()
		for _, g := range gs {
			gid, err := database.InsertID(q, `INSERT INTO option_groups(menu_item_id,name,min_choices,max_choices)
				SELECT ?,name,min_choices,max_choices FROM option_groups WHERE id=?`, id, g)
			if err != nil {
				return err
			}
			groups[g] = gid
		}
		type opt struct {
			id    int64
			group sql.NullInt64
		}
		var os []opt
		orows, err := q.Query(`SELECT id,group_id FROM item_options WHERE menu_item_id=? ORDER BY id`, it.old)
		if err != nil {
			return err
		}
		for orows.Next() {
			var o opt
			orows.Scan(&o.id, &o.group)
			os = append(os, o)
		}
		orows.Close()
		for _, o := range os {
			var group any
			if o.group.Valid {
				group = groups[o.group.Int64]
			}
			if err := exec(q, `INSERT INTO item_options(menu_item_id,name,extra_price,group_id)
				SELECT ?,name,extra_price,? FROM item_options WHERE id=?`, id, group, o.id); err != nil {
				return err
			}
		}
	}
	return nil
}

// branchUpdate changes a branch's name, invoice code, address and phone.
func (a *App) branchUpdate(w http.ResponseWriter, r *http.Request, s *Staff) {
	id := pathID(r, "id")
	name := clean(r.FormValue("branch_name"), 40)
	code, ok := branchCode(r.FormValue("code"))
	if !ok {
		branchErr(w, r, "The code can have up to 6 letters and digits, for example GUL.")
		return
	}
	if a.codeTaken(s.R.BusinessID, id, code) {
		branchErr(w, r, "Another branch already uses the code "+code+".")
		return
	}
	res, err := a.db.Exec(`UPDATE restaurants SET branch_name=?,branch_code=?,address=?,phone=? WHERE id=? AND business_id=?`,
		name, code, clean(r.FormValue("address"), 120), clean(r.FormValue("phone"), 20), id, s.R.BusinessID)
	if rows(res, err) != 1 {
		http.NotFound(w, r)
		return
	}
	a.audit(id, s.Name, "branch changed", strings.TrimSpace(name+" "+code))
	back(w, r, "/owner/branches?done=saved")
}

// branchStatus sets a branch active, closed for now (guests can't order, staff still log in) or
// archived (hidden, its staff can't log in, its QR codes stop working; data is kept).
func (a *App) branchStatus(w http.ResponseWriter, r *http.Request, s *Staff) {
	id, status := pathID(r, "id"), r.FormValue("status")
	if status != "active" && status != "closed" && status != "archived" {
		http.Error(w, "unknown status", http.StatusBadRequest)
		return
	}
	var cur, name string
	if err := a.db.QueryRow(`SELECT status,CASE WHEN branch_name!='' THEN branch_name ELSE name END FROM restaurants WHERE id=? AND business_id=?`,
		id, s.R.BusinessID).Scan(&cur, &name); err != nil {
		http.NotFound(w, r)
		return
	}
	if cur == status {
		back(w, r, "/owner/branches")
		return
	}
	if status == "archived" {
		var open bool
		a.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM table_sessions WHERE restaurant_id=? AND status IN ('open','bill_requested'))`, id).Scan(&open)
		if open {
			branchErr(w, r, name+" still has open bills. Close them before archiving the branch.")
			return
		}
		if len(s.Branches) <= 1 {
			branchErr(w, r, "This is your only branch in use, so it can't be archived.")
			return
		}
	}
	if cur == "archived" {
		if full, max := a.atBranchLimit(s.R.BusinessID); full {
			branchErr(w, r, fmt.Sprintf("Your plan allows %d branches in use. Archive another branch first, or ask Bosho to raise the limit.", max))
			return
		}
	}
	err := a.inTx(func(q database.Querier) error {
		if err := exec(q, `UPDATE restaurants SET status=? WHERE id=?`, status, id); err != nil {
			return err
		}
		if status == "archived" { // its staff are logged out; the owner's logins move off it on the next page
			return exec(q, `DELETE FROM staff_sessions WHERE staff_id IN (SELECT id FROM staff_users WHERE restaurant_id=? AND role!='owner')`, id)
		}
		return nil
	})
	if err != nil {
		serverError(w, err)
		return
	}
	a.audit(id, s.Name, "branch "+status, name)
	back(w, r, "/owner/branches?done="+status)
}
