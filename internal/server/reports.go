package server

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"bosho/internal/database"
	"bosho/internal/money"
)

// localDay is the SQL for the restaurant-local date of a Unix-seconds column.
func localDay(col string) string {
	return `to_char(to_timestamp(` + col + `) AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD')`
}

type dayRow struct {
	Day   string
	Bills int
	Sales int64
	Width int // bar length in percent of the best day
}

type dishRow struct {
	Name    string
	Qty     int
	Revenue int64
	Width   int
}

type hourRow struct {
	Hour   int
	Label  string
	Orders int
	Width  int
}

// reports: sales per day, per dish and per hour over the last 7, 30 or 90 days.
func (a *App) reports(w http.ResponseWriter, r *http.Request, s *Staff) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days != 7 && days != 90 {
		days = 30
	}
	t := time.Now().In(a.loc)
	from := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, a.loc).AddDate(0, 0, -(days - 1)).Unix()
	rid := s.R.ID

	daily := database.List(a.db, `SELECT `+localDay("created_at")+` AS d,COUNT(*),SUM(amount)::BIGINT FROM payments
		WHERE restaurant_id=? AND created_at>=? GROUP BY d ORDER BY d DESC`,
		func(rs *sql.Rows, d *dayRow) error { return rs.Scan(&d.Day, &d.Bills, &d.Sales) }, rid, from)
	var total, best int64
	bills := 0
	for _, d := range daily {
		total += d.Sales
		bills += d.Bills
		best = max(best, d.Sales)
	}
	for i := range daily {
		if best > 0 {
			daily[i].Width = int(daily[i].Sales * 100 / best)
		}
	}

	dishes := database.List(a.db, `SELECT CASE WHEN i.name_en!='' THEN i.name_en ELSE i.name_bn END,SUM(i.quantity),SUM(i.quantity*i.unit_price)::BIGINT
		FROM order_items i JOIN orders o ON o.id=i.order_id
		WHERE o.restaurant_id=? AND o.placed_at>=? AND o.status NOT IN ('rejected','cancelled') AND i.status!='cancelled'
		GROUP BY i.menu_item_id,i.name_en,i.name_bn ORDER BY 3 DESC`,
		func(rs *sql.Rows, d *dishRow) error { return rs.Scan(&d.Name, &d.Qty, &d.Revenue) }, rid, from)
	if len(dishes) > 0 && dishes[0].Revenue > 0 {
		for i := range dishes {
			dishes[i].Width = int(dishes[i].Revenue * 100 / dishes[0].Revenue)
		}
	}

	hours := database.List(a.db, `SELECT EXTRACT(HOUR FROM to_timestamp(placed_at) AT TIME ZONE 'Asia/Dhaka')::INT AS h,COUNT(*)
		FROM orders WHERE restaurant_id=? AND placed_at>=? AND status NOT IN ('rejected','cancelled') GROUP BY h ORDER BY h`,
		func(rs *sql.Rows, h *hourRow) error { return rs.Scan(&h.Hour, &h.Orders) }, rid, from)
	busiest := 0
	for _, h := range hours {
		busiest = max(busiest, h.Orders)
	}
	for i := range hours {
		hours[i].Label = clock12(hours[i].Hour * 60)
		if busiest > 0 {
			hours[i].Width = hours[i].Orders * 100 / busiest
		}
	}
	a.render(w, "owner_reports", M{"Staff": s, "Page": "reports", "Days": days, "Daily": daily, "Total": total, "Bills": bills,
		"Dishes": dishes, "Hours": hours, "Today": t.Format("2006-01-02"), "From": time.Unix(from, 0).In(a.loc).Format("2006-01-02")})
}

type removedRow struct {
	Who, Item, Reason, Table string
	Qty                      int
	Amount                   int64
}

// closing is the end-of-day report: money by payment method to match against the drawer, items
// removed from bills, orders rejected, groups closed without payment, and bills still open.
func (a *App) closing(w http.ResponseWriter, r *http.Request, s *Staff) {
	day, err := time.ParseInLocation("2006-01-02", r.URL.Query().Get("day"), a.loc)
	if err != nil {
		t := time.Now().In(a.loc)
		day = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, a.loc)
	}
	start, end := day.Unix(), day.AddDate(0, 0, 1).Unix()
	rid := s.R.ID
	methods := database.List(a.db, `SELECT method,COUNT(*),SUM(amount)::BIGINT FROM payments WHERE restaurant_id=? AND created_at>=? AND created_at<? GROUP BY method ORDER BY 3 DESC`,
		func(rs *sql.Rows, m *methodRow) error { return rs.Scan(&m.Method, &m.Count, &m.Amount) }, rid, start, end)
	var total int64
	bills := 0
	for _, m := range methods {
		total += m.Amount
		bills += m.Count
	}
	removed := database.List(a.db, `SELECT i.cancelled_by,CASE WHEN i.name_en!='' THEN i.name_en ELSE i.name_bn END,i.cancel_reason,t.label,i.quantity,(i.quantity*i.unit_price)::BIGINT
		FROM order_items i JOIN orders o ON o.id=i.order_id JOIN table_sessions s ON s.id=o.session_id JOIN tables t ON t.id=s.table_id
		WHERE o.restaurant_id=? AND o.placed_at>=? AND o.placed_at<? AND i.status='cancelled'
		  AND i.cancel_reason NOT IN ('guest cancelled before accept','guest request') ORDER BY i.id`,
		func(rs *sql.Rows, x *removedRow) error {
			return rs.Scan(&x.Who, &x.Item, &x.Reason, &x.Table, &x.Qty, &x.Amount)
		}, rid, start, end)
	var rejected int
	a.db.QueryRow(`SELECT COUNT(*) FROM orders WHERE restaurant_id=? AND placed_at>=? AND placed_at<? AND status='rejected'`, rid, start, end).Scan(&rejected)
	unpaid := database.List(a.db, `SELECT staff_name,action,detail,created_at FROM audit_log
		WHERE restaurant_id=? AND action='closed without payment' AND created_at>=? AND created_at<? ORDER BY id`,
		func(rs *sql.Rows, x *auditRow) error {
			var at int64
			err := rs.Scan(&x.Who, &x.Action, &x.Detail, &at)
			x.At = time.Unix(at, 0).In(a.loc).Format("3:04 pm")
			return err
		}, rid, start, end)
	var openBills int
	a.db.QueryRow(`SELECT COUNT(*) FROM table_sessions WHERE restaurant_id=? AND status IN ('open','bill_requested')`, rid).Scan(&openBills)
	a.render(w, "owner_closing", M{"Staff": s, "Page": "reports", "Day": day.Format("2006-01-02"), "DayLong": day.Format("Monday, 2 January 2006"),
		"Methods": methods, "Total": total, "Bills": bills, "Removed": removed, "Rejected": rejected, "Unpaid": unpaid, "OpenBills": openBills})
}

// export downloads orders, payments or reviews as a CSV file for a spreadsheet or an accountant.
// The range is ?from=YYYY-MM-DD&to=YYYY-MM-DD (both days included), the last 30 days by default.
func (a *App) export(w http.ResponseWriter, r *http.Request, s *Staff) {
	q := r.URL.Query()
	t := time.Now().In(a.loc)
	today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, a.loc)
	from, err := time.ParseInLocation("2006-01-02", q.Get("from"), a.loc)
	if err != nil {
		from = today.AddDate(0, 0, -29)
	}
	to, err := time.ParseInLocation("2006-01-02", q.Get("to"), a.loc)
	if err != nil {
		to = today
	}
	start, end := from.Unix(), to.AddDate(0, 0, 1).Unix()
	what := r.PathValue("what")
	var head []string
	var query string
	switch what {
	case "orders.csv":
		head = []string{"time", "table", "group", "order", "order status", "dish", "options", "note", "quantity", "unit price", "line total", "item status", "removed by", "reason", "placed by"}
		query = `SELECT o.placed_at,t.label,s.group_label,o.id::TEXT,o.status,CASE WHEN i.name_en!='' THEN i.name_en ELSE i.name_bn END,i.options_text,i.note,
			i.quantity::TEXT,i.unit_price,(i.quantity*i.unit_price)::BIGINT,i.status,i.cancelled_by,i.cancel_reason,o.placed_by
			FROM order_items i JOIN orders o ON o.id=i.order_id JOIN table_sessions s ON s.id=o.session_id JOIN tables t ON t.id=s.table_id
			WHERE o.restaurant_id=? AND o.placed_at>=? AND o.placed_at<? ORDER BY o.id,i.id`
	case "payments.csv":
		head = []string{"time", "table", "group", "amount", "method", "marked by"}
		query = `SELECT p.created_at,t.label,s.group_label,p.amount,p.method,p.marked_by
			FROM payments p JOIN table_sessions s ON s.id=p.session_id JOIN tables t ON t.id=s.table_id
			WHERE p.restaurant_id=? AND p.created_at>=? AND p.created_at<? ORDER BY p.id`
	case "reviews.csv":
		head = []string{"time", "table", "dish", "rating", "comment"}
		query = `SELECT v.created_at,t.label,COALESCE((SELECT CASE WHEN m.name_en!='' THEN m.name_en ELSE m.name_bn END FROM menu_items m WHERE m.id=v.menu_item_id),'(overall)'),
			v.rating::TEXT,v.comment
			FROM reviews v JOIN table_sessions s ON s.id=v.session_id JOIN tables t ON t.id=s.table_id
			WHERE v.restaurant_id=? AND v.created_at>=? AND v.created_at<? ORDER BY v.id`
	default:
		http.NotFound(w, r)
		return
	}
	rs, err := a.db.Query(query, s.R.ID, start, end)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rs.Close()
	name := fmt.Sprintf("bosho-%s-%s-to-%s", what[:len(what)-4], from.Format("2006-01-02"), to.Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
	w.Write([]byte("\xEF\xBB\xBF")) // byte order mark: Excel then reads the Bangla correctly
	cw := csv.NewWriter(w)
	cw.Write(head)
	cols, _ := rs.ColumnTypes()
	for rs.Next() {
		vals := make([]any, len(cols))
		for i := range vals {
			vals[i] = new(any)
		}
		if err := rs.Scan(vals...); err != nil {
			break
		}
		rec := make([]string, len(vals))
		for i, v := range vals {
			switch x := (*v.(*any)).(type) {
			case int64: // column 0 is a time, BIGINT money columns are poisha
				if i == 0 {
					rec[i] = time.Unix(x, 0).In(a.loc).Format("2006-01-02 15:04")
				} else {
					rec[i] = money.Taka(x)
				}
			case nil:
				rec[i] = ""
			default:
				rec[i] = fmt.Sprint(x)
			}
		}
		cw.Write(rec)
	}
	cw.Flush()
}
