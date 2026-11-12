package money

import "testing"

func TestParseAndFormat(t *testing.T) {
	cases := []struct {
		in     string
		poisha int64
		out    string
	}{
		{"120", 12000, "৳120"},
		{"120.5", 12050, "৳120.50"},
		{" 99.99 ", 9999, "৳99.99"},
		{"abc", 0, "৳0"},
		{"-5", 0, "৳0"},
	}
	for _, c := range cases {
		if got := Parse(c.in); got != c.poisha {
			t.Errorf("Parse(%q) = %d, want %d", c.in, got, c.poisha)
		}
		if got := Format(c.poisha); got != c.out {
			t.Errorf("Format(%d) = %q, want %q", c.poisha, got, c.out)
		}
	}
}

func TestPercent(t *testing.T) {
	if got := Percent(56000, 10); got != 5600 {
		t.Errorf("10%% of 560 taka = %d, want 5600", got)
	}
	if got := Percent(3333, 5); got != 167 {
		t.Errorf("rounding: got %d, want 167", got)
	}
}
