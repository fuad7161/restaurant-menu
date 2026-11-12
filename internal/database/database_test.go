package database

import "testing"

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
