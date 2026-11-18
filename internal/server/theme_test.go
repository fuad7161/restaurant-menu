package server

import (
	"strings"
	"testing"
)

func TestSwatchesAreReadable(t *testing.T) {
	for _, s := range Swatches {
		if !usableColor(s.Hex) {
			t.Errorf("swatch %s %s is too light for white text", s.Name, s.Hex)
		}
	}
}

func TestUsableColor(t *testing.T) {
	for in, want := range map[string]bool{
		"#8f1d21": true, "#000000": true, "#FFFFFF": false, "#f2c94c": false,
		"8f1d21": false, "#8f1d2": false, "red": false, "#8f1d21;}body{": false, "": false,
	} {
		if got := usableColor(in); got != want {
			t.Errorf("usableColor(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestThemeCSSFallsBack(t *testing.T) {
	if got := string(themeCSS("</style><script>")); !strings.Contains(got, "--brand:"+DefaultColor) {
		t.Errorf("bad colour should fall back to the default, got %s", got)
	}
	if got := string(themeCSS("#1f6f4a")); !strings.Contains(got, "--brand:#1f6f4a;") {
		t.Errorf("theme lost the colour: %s", got)
	}
}
