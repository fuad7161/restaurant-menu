package server

import (
	"database/sql"
	"net/http"
	"time"

	"bosho/internal/database"
)

// audit records who did something that touches money or access: a price change, an item
// removed from a bill, a payment, a group closed without payment, staff changes. Owners read it
// on the Activity page. A failed write is logged by the database package and never blocks the action.
func (a *App) audit(rid int64, who, action, detail string) {
	a.db.Exec(`INSERT INTO audit_log(restaurant_id,business_id,staff_name,action,detail,created_at)
		VALUES(?,(SELECT business_id FROM restaurants WHERE id=?),?,?,?,?)`,
		rid, rid, who, action, clean(detail, 400), now())
}

type auditRow struct {
	Who, Action, Detail, At string
	Branch                  string
}

// activity shows the latest 300 entries of the audit log.
func (a *App) activity(w http.ResponseWriter, r *http.Request, s *Staff) {
	// One branch, or in "All branches" mode every branch of the business with a branch column.
	where, arg := scope(s, "l.restaurant_id")
	rows := database.List(a.db, `SELECT l.staff_name,l.action,l.detail,l.created_at,`+branchLabelSQL+`
		FROM audit_log l JOIN restaurants rb ON rb.id=l.restaurant_id WHERE `+where+` ORDER BY l.id DESC LIMIT 300`,
		func(rs *sql.Rows, x *auditRow) error {
			var at int64
			err := rs.Scan(&x.Who, &x.Action, &x.Detail, &at, &x.Branch)
			x.At = time.Unix(at, 0).In(a.loc).Format("2 Jan, 3:04 pm")
			return err
		}, arg)
	a.render(w, "owner_activity", M{"Staff": s, "Page": "activity", "Rows": rows})
}
