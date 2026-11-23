package server

import (
	"context"
	"log/slog"
	"time"
)

// maintainEvery is how often the cleanup runs. Each run also queries the database, which keeps
// a free Supabase project from pausing for inactivity while the app is running.
const maintainEvery = 10 * time.Minute

// Maintain cleans up in the background until ctx ends. main starts it once.
func (a *App) Maintain(ctx context.Context) {
	a.cleanup()
	t := time.NewTicker(maintainEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.cleanup()
		}
	}
}

// cleanup closes what was left behind, so the counter never fills with dead bills:
//   - groups that never ordered and have been idle for sessionIdle (the guest screens do this
//     per table on the next scan; this catches tables nobody scans again);
//   - carts of groups that are no longer open;
//   - waiter calls nobody answered within 3 hours;
//   - expired staff logins and old rate-limit counters.
//
// Groups with orders are never closed here: they hold money, and the counter screen shows how
// long they have been open.
func (a *App) cleanup() {
	n := now()
	steps := []struct {
		what  string
		query string
		args  []any
	}{
		{"idle groups", `UPDATE table_sessions SET status='closed', closed_at=? WHERE status='open' AND last_activity<?
			AND NOT EXISTS (SELECT 1 FROM orders o WHERE o.session_id=table_sessions.id AND o.status NOT IN ('rejected','cancelled'))`,
			[]any{n, n - sessionIdle}},
		{"old carts", `DELETE FROM cart_items c USING table_sessions s WHERE s.id=c.session_id AND s.status NOT IN ('open','bill_requested')`, nil},
		{"old calls", `UPDATE waiter_calls SET status='done', done_at=? WHERE status='pending' AND created_at<?`, []any{n, n - 3*3600}},
		{"expired logins", `DELETE FROM staff_sessions WHERE expires_at<?`, []any{n}},
		{"old email links", `DELETE FROM email_tokens WHERE expires_at<?`, []any{n - 7*86400}},
	}
	changed := false
	for _, s := range steps {
		res, err := a.db.Exec(s.query, s.args...)
		if err != nil {
			slog.Error("cleanup", "step", s.what, "error", err)
			continue
		}
		if k, _ := res.RowsAffected(); k > 0 {
			slog.Info("cleanup", "step", s.what, "rows", k)
			changed = true
		}
	}
	if changed {
		a.changed()
	}
	a.limits.sweep(time.Hour)
}
