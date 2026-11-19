package server

import (
	"net/http"
	"strings"
)

// ownerTables lists the tables. Switched-off tables are hidden unless ?off=1 asks for them.
func (a *App) ownerTables(w http.ResponseWriter, r *http.Request, s *Staff) {
	showOff := r.URL.Query().Get("off") == "1"
	var shown []tableRow
	hidden := 0
	for _, t := range a.tablesWithGroups(s.R.ID) {
		if !t.Active && !showOff {
			hidden++
			continue
		}
		shown = append(shown, t)
	}
	a.render(w, "owner_tables", M{"Staff": s, "Page": "qr", "Tables": shown, "Hidden": hidden, "ShowOff": showOff,
		"Base": a.base(r), "Renewed": queryID(r, "renewed")})
}

// tablesBack returns to the tables page, keeping switched-off tables visible if they were.
func tablesBack(w http.ResponseWriter, r *http.Request, extra string) {
	to := "/owner/tables"
	if r.FormValue("off") == "1" {
		extra = "off=1&" + extra
	}
	if extra = strings.TrimSuffix(extra, "&"); extra != "" {
		to += "?" + extra
	}
	back(w, r, to)
}

func (a *App) tableCreate(w http.ResponseWriter, r *http.Request, s *Staff) {
	if label := clean(r.FormValue("label"), 12); label != "" {
		// The QR carries a random token, never the table number, so it cannot be guessed.
		a.db.Exec(`INSERT INTO tables(restaurant_id,label,qr_token) VALUES(?,?,?)`, s.R.ID, label, randToken(8))
	}
	back(w, r, "/owner/tables")
}

func (a *App) tableToggle(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.db.Exec(`UPDATE tables SET is_active=NOT is_active WHERE id=? AND restaurant_id=?`, pathID(r, "id"), s.R.ID)
	tablesBack(w, r, "")
}

// tableNewQR gives the table a fresh random token. The old QR (and any photo of it) stops working
// at once; guests seated at the table keep their group but must scan the new card.
func (a *App) tableNewQR(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.db.Exec(`UPDATE tables SET qr_token=? WHERE id=? AND restaurant_id=?`, randToken(8), pathID(r, "id"), s.R.ID)
	tablesBack(w, r, "renewed="+r.PathValue("id"))
}

func (a *App) tableQR(w http.ResponseWriter, r *http.Request, s *Staff) {
	var tok string
	if a.db.QueryRow(`SELECT qr_token FROM tables WHERE id=? AND restaurant_id=?`, pathID(r, "id"), s.R.ID).Scan(&tok) != nil {
		http.NotFound(w, r)
		return
	}
	png, err := a.qrImage(r, s.R, a.base(r)+"/t/"+tok)
	if err != nil {
		http.Error(w, "qr error", 500)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache") // the same URL changes with a new QR, logo or colour
	w.Write(png)
}

// tablePrint shows the QR cards ready to print: all active tables, or only ?id= (opened by a
// card's Print button, which also opens the print dialog).
func (a *App) tablePrint(w http.ResponseWriter, r *http.Request, s *Staff) {
	ts := a.tablesWithGroups(s.R.ID)
	one := queryID(r, "id")
	if one != 0 {
		var only []tableRow
		for _, t := range ts {
			if t.ID == one {
				only = append(only, t)
			}
		}
		if len(only) == 0 {
			http.NotFound(w, r)
			return
		}
		ts = only
	}
	a.render(w, "owner_print", M{"Staff": s, "Tables": ts, "One": one != 0})
}
