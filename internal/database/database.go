// Package database opens PostgreSQL, runs migrations, and offers small query helpers.
//
// Queries in this codebase are written with "?" placeholders. DB and Tx rewrite
// them to PostgreSQL's "$1, $2, ..." form, so no query may contain a literal "?".
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Querier is satisfied by both DB and Tx, so a query helper can run inside or outside a transaction.
type Querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}

type DB struct{ pool *sql.DB }

type Tx struct{ tx *sql.Tx }

// Open connects to PostgreSQL, waiting up to a minute for it to accept connections.
func Open(url string) (*DB, error) {
	pool, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(20)
	pool.SetConnMaxIdleTime(5 * time.Minute)
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = pool.PingContext(ctx)
		cancel()
		if err == nil {
			return &DB{pool}, nil
		}
		if attempt == 30 {
			return nil, fmt.Errorf("database not reachable: %w", err)
		}
		log.Printf("waiting for database (%v)", err)
		time.Sleep(2 * time.Second)
	}
}

func (d *DB) Close() error { return d.pool.Close() }

func (d *DB) Query(q string, args ...any) (*sql.Rows, error) { return d.pool.Query(rebind(q), args...) }
func (d *DB) QueryRow(q string, args ...any) *sql.Row        { return d.pool.QueryRow(rebind(q), args...) }
func (d *DB) Exec(q string, args ...any) (sql.Result, error) { return d.pool.Exec(rebind(q), args...) }

func (d *DB) Begin() (*Tx, error) {
	tx, err := d.pool.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{tx}, nil
}

func (t *Tx) Query(q string, args ...any) (*sql.Rows, error) { return t.tx.Query(rebind(q), args...) }
func (t *Tx) QueryRow(q string, args ...any) *sql.Row        { return t.tx.QueryRow(rebind(q), args...) }
func (t *Tx) Exec(q string, args ...any) (sql.Result, error) { return t.tx.Exec(rebind(q), args...) }
func (t *Tx) Commit() error                                  { return t.tx.Commit() }
func (t *Tx) Rollback() error                                { return t.tx.Rollback() }

// rebind turns "?" placeholders into "$1", "$2", ...
func rebind(q string) string {
	if !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteByte(q[i])
		}
	}
	return b.String()
}

// InsertID runs an INSERT and returns the new row's id.
func InsertID(q Querier, query string, args ...any) (int64, error) {
	var id int64
	err := q.QueryRow(query+" RETURNING id", args...).Scan(&id)
	return id, err
}

// List runs a query and collects every row with the given scan function.
func List[T any](q Querier, query string, scan func(*sql.Rows, *T) error, args ...any) []T {
	rows, err := q.Query(query, args...)
	if err != nil {
		log.Println("query:", err)
		return nil
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var v T
		if err := scan(rows, &v); err != nil {
			log.Println("scan:", err)
			return out
		}
		out = append(out, v)
	}
	return out
}

// Migrate applies every migrations/*.sql file that has not run yet, in file-name order.
func (d *DB) Migrate() error {
	if _, err := d.pool.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		var done bool
		if err := d.pool.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, e.Name()).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}
		tx, err := d.pool.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", e.Name(), err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(name) VALUES($1)`, e.Name()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Printf("applied migration %s", e.Name())
	}
	return nil
}
