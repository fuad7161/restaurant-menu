package server

import (
	"fmt"
	"html/template"
	"math"
	"regexp"
	"strconv"
)

// DefaultColor is the brand colour a new restaurant starts with (sindoor maroon).
const DefaultColor = "#8f1d21"

// Swatches are the ready-made brand colours offered on the profile page.
// Every one is dark enough for white text on it and for coloured text on the cream page.
var Swatches = []struct{ Name, Hex string }{
	{"Sindoor", "#8f1d21"}, {"Chili", "#b3261e"}, {"Terracotta", "#a0461b"}, {"Mustard", "#8a5a00"},
	{"Banana leaf", "#1f6f4a"}, {"Mint", "#0f766e"}, {"Indigo", "#3730a3"}, {"Jamdani blue", "#1d4e89"},
	{"Plum", "#6b2160"}, {"Royal purple", "#6d28d9"}, {"Charcoal", "#2f2a28"}, {"Rose", "#9f1239"},
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type rgb struct{ r, g, b float64 }

func parseHex(s string) (rgb, bool) {
	if !hexColor.MatchString(s) {
		return rgb{}, false
	}
	v, _ := strconv.ParseUint(s[1:], 16, 32)
	return rgb{float64(v >> 16 & 255), float64(v >> 8 & 255), float64(v & 255)}, true
}

func (c rgb) hex() string {
	cl := func(x float64) int { return int(math.Round(math.Max(0, math.Min(255, x)))) }
	return fmt.Sprintf("#%02x%02x%02x", cl(c.r), cl(c.g), cl(c.b))
}

// mix returns c moved toward o by t (0 = c, 1 = o).
func (c rgb) mix(o rgb, t float64) rgb {
	return rgb{c.r + (o.r-c.r)*t, c.g + (o.g-c.g)*t, c.b + (o.b-c.b)*t}
}

// luminance is the WCAG relative luminance, 0 (black) to 1 (white).
func (c rgb) luminance() float64 {
	ch := func(v float64) float64 {
		v /= 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.r) + 0.7152*ch(c.g) + 0.0722*ch(c.b)
}

// usableColor reports whether a brand colour keeps text readable: white text on it and
// coloured text on the white page both need a contrast of about 3:1 or more.
func usableColor(s string) bool {
	c, ok := parseHex(s)
	return ok && c.luminance() <= 0.27
}

// themeCSS turns a restaurant's brand colour into the accent variables app.css reads: the colour
// itself (buttons, links, selected states), a darker shade for hover, and a light tint for
// selected backgrounds. app.css also mixes a little of it into the page background.
// Anything invalid falls back to the default, so the output is always safe to put in a <style>.
func themeCSS(color string) template.CSS {
	if !usableColor(color) {
		color = DefaultColor
	}
	c, _ := parseHex(color)
	white, black := rgb{255, 255, 255}, rgb{0, 0, 0}
	return template.CSS(fmt.Sprintf(":root{--brand:%s;--brand-deep:%s;--brand-soft:%s}",
		c.hex(), c.mix(black, .2).hex(), c.mix(white, .9).hex()))
}

// themeColor is the colour of the phone's browser bar (<meta name="theme-color">): white, to
// match the white page header.
func themeColor(color string) string { return "#ffffff" }
