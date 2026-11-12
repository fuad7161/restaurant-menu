package server

import (
	"net/http"

	"github.com/skip2/go-qrcode"
)

func (a *App) ownerTables(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "owner_tables", M{"Staff": s, "Page": "qr", "Tables": a.tablesWithGroups(s.R.ID), "Base": a.base(r)})
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
	back(w, r, "/owner/tables")
}

func (a *App) tableQR(w http.ResponseWriter, r *http.Request, s *Staff) {
	var tok string
	if a.db.QueryRow(`SELECT qr_token FROM tables WHERE id=? AND restaurant_id=?`, pathID(r, "id"), s.R.ID).Scan(&tok) != nil {
		http.NotFound(w, r)
		return
	}
	png, err := qrcode.Encode(a.base(r)+"/t/"+tok, qrcode.Medium, 640)
	if err != nil {
		http.Error(w, "qr error", 500)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

func (a *App) tablePrint(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "owner_print", M{"Staff": s, "Tables": a.tablesWithGroups(s.R.ID)})
}
