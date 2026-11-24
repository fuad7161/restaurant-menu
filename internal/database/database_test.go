package database

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRebind(t *testing.T) {
	got := rebind(`UPDATE t SET a=?, b=? WHERE id=?`)
	want := `UPDATE t SET a=$1, b=$2 WHERE id=$3`
	if got != want {
		t.Errorf("rebind = %q, want %q", got, want)
	}
	if q := `SELECT 1`; rebind(q) != q {
		t.Error("a query without placeholders must not change")
	}
}

// TestBranchesMigration loads two restaurants into the schema before 0008_branches.sql and checks
// that each becomes its own business with one branch, keeping its data. It needs TEST_DATABASE_URL.
func TestBranchesMigration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	schema := fmt.Sprintf("bosho_mig_%d", time.Now().UnixNano())
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		admin.Close()
	})
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := Open(dsn + sep + "search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.migrate("0008"); err != nil {
		t.Fatal(err)
	}

	// Two restaurants; the second has a table and a paid bill.
	r1, _ := InsertID(db, `INSERT INTO restaurants(name,slug,logo,brand_color,tagline,created_at) VALUES('Kacchi Ghar','kg','kg.webp','#123456','Since 1990',100)`)
	r2, _ := InsertID(db, `INSERT INTO restaurants(name,slug,created_at) VALUES('Star Kabab','sk',200)`)
	tid, _ := InsertID(db, `INSERT INTO tables(restaurant_id,label,qr_token) VALUES(?,'1','tok')`, r2)
	if _, err := db.Exec(`INSERT INTO table_sessions(restaurant_id,table_id,group_label,status,opened_at,last_activity) VALUES(?,?,'A','closed',1,1)`, r2, tid); err != nil {
		t.Fatal(err)
	}

	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	var businesses int
	db.QueryRow(`SELECT COUNT(*) FROM businesses`).Scan(&businesses)
	if businesses != 2 {
		t.Fatalf("%d businesses, want 2", businesses)
	}
	for _, c := range []struct {
		rid                        int64
		name, logo, color, tagline string
	}{{r1, "Kacchi Ghar", "kg.webp", "#123456", "Since 1990"}, {r2, "Star Kabab", "", "#8f1d21", ""}} {
		var name, logo, color, tagline, status string
		var branches int
		err := db.QueryRow(`SELECT b.name,b.logo,b.brand_color,b.tagline,r.status,
			(SELECT COUNT(*) FROM restaurants x WHERE x.business_id=b.id)
			FROM restaurants r JOIN businesses b ON b.id=r.business_id WHERE r.id=?`, c.rid).
			Scan(&name, &logo, &color, &tagline, &status, &branches)
		if err != nil {
			t.Fatalf("restaurant %d: %v", c.rid, err)
		}
		if name != c.name || logo != c.logo || color != c.color || tagline != c.tagline || status != "active" || branches != 1 {
			t.Errorf("restaurant %d became %q %q %q %q, %s, %d branches", c.rid, name, logo, color, tagline, status, branches)
		}
	}
	var sessions int
	db.QueryRow(`SELECT COUNT(*) FROM table_sessions WHERE restaurant_id=?`, r2).Scan(&sessions)
	if sessions != 1 {
		t.Errorf("Star Kabab has %d bills after the migration, want 1", sessions)
	}
}
