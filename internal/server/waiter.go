package server

import (
	"database/sql"
	"net/http"
	"strconv"

	"bosho/internal/database"
)

type tableRow struct {
	Table
	Groups []groupRow
}

func (a *App) tablesWithGroups(rid int64) []tableRow {
	ts := database.List(a.db, `SELECT id,label,qr_token,is_active FROM tables WHERE restaurant_id=? ORDER BY label`,
		func(r *sql.Rows, t *tableRow) error { return r.Scan(&t.ID, &t.Label, &t.Token, &t.Active) }, rid)
	for i := range ts {
		ts[i].Groups = a.openGroups(ts[i].ID)
	}
	return ts
}

func (a *App) staffTables(w http.ResponseWriter, r *http.Request, s *Staff) {
	a.render(w, "staff_tables", M{"Staff": s, "Tables": a.tablesWithGroups(s.R.ID), "Page": "tables"})
}

// staffOpenTable makes the waiter's phone a member of a group, then hands over to the normal guest screens.
func (a *App) staffOpenTable(w http.ResponseWriter, r *http.Request, s *Staff) {
	var t Table
	err := a.db.QueryRow(`SELECT id,qr_token FROM tables WHERE id=? AND restaurant_id=? AND is_active`, pathID(r, "id"), s.R.ID).Scan(&t.ID, &t.Token)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	dev := deviceToken(w, r)
	name := "Waiter " + s.Name
	sid, _ := strconv.ParseInt(r.FormValue("session"), 10, 64)
	var okSession int
	a.db.QueryRow(`SELECT COUNT(*) FROM table_sessions WHERE id=? AND table_id=? AND status IN ('open','bill_requested')`, sid, t.ID).Scan(&okSession)
	if okSession == 0 {
		a.newSession(s.R.ID, t.ID, name, dev, true)
	} else {
		res, _ := a.db.Exec(`UPDATE session_guests SET status='joined', is_staff=TRUE, last_seen=? WHERE session_id=? AND device_token=?`, now(), sid, dev)
		if n, _ := res.RowsAffected(); n == 0 {
			a.db.Exec(`INSERT INTO session_guests(session_id,display_name,device_token,status,is_staff,last_seen) VALUES(?,?,?,'joined',TRUE,?)`, sid, name, dev, now())
		}
	}
	http.Redirect(w, r, "/t/"+t.Token, http.StatusSeeOther)
}
