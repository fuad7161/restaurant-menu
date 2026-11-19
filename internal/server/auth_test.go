package server

import "testing"

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/owner/tables":         "/owner/tables",
		"/staff/kitchen?x=1":    "/staff/kitchen?x=1",
		"":                      "",
		"/":                     "",
		"https://evil.com":      "",
		"//evil.com":            "",
		`/\evil.com`:            "",
		"/login?next=/owner":    "",
		"/owner\r\nSet-Cookie:": "",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}
