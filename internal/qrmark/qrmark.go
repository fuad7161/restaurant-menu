// Package qrmark draws table QR codes with the restaurant's mark in the middle: its logo, or the
// first letter of its name on the brand colour, like the round mark on the guest screens.
//
// The code uses the highest error correction level (H), which can rebuild up to 30% of the
// symbol, so the badge (about 6% of the symbol's area) never stops it from scanning.
package qrmark

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // logo formats accepted by the upload
	"image/png"
	"math"

	"github.com/skip2/go-qrcode"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
)

// Tiro Bangla is the display font of the app (the round mark uses it), and it has both Bangla and
// Latin letters. SIL Open Font License, see OFL.txt.
//
//go:embed TiroBangla-Regular.ttf
var tiroBangla []byte

var markFont, _ = opentype.Parse(tiroBangla)

// badgeShare is the badge diameter as a share of the symbol width (quiet zone excluded).
const badgeShare = 0.28

// DecodeLogo reads an uploaded JPEG, PNG or WebP logo.
func DecodeLogo(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// Render returns a size×size PNG of a QR code for url: data modules as round dots, the three corner
// "eyes" as rounded squares, and a round badge in the middle with the logo, or letter in white on
// brand when logo is nil.
//
// Dots and eyes stay black. Phones find a code by its eyes, and a coloured eye (even only its
// centre) halved the scan rate on dim, tilted, noisy test photos, while black round dots scanned
// as well as plain squares. The brand colour lives in the badge, which is safe to colour.
func Render(url string, size int, logo image.Image, letter string, brand color.Color) ([]byte, error) {
	q, err := qrcode.New(url, qrcode.Highest)
	if err != nil {
		return nil, err
	}
	bits := q.Bitmap() // includes the 4-module quiet zone on each side
	n := len(bits)
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)

	m := float64(size) / float64(n) // module size in pixels
	c := float64(size) / 2
	outer := m * float64(n-8) * badgeShare / 2 // badge with its white ring
	inner := outer * 0.86

	ink := image.Black

	inEye := func(x, y int) bool {
		x, y = x-4, y-4 // symbol coordinates
		last := n - 8 - 7
		return (x < 7 && y < 7) || (x >= last && y < 7) || (x < 7 && y >= last)
	}
	for y := range n {
		for x := range n {
			if !bits[y][x] || inEye(x, y) {
				continue
			}
			mx, my := (float64(x)+.5)*m, (float64(y)+.5)*m
			if math.Hypot(mx-c, my-c) < outer+m*.3 { // hidden behind the badge anyway
				continue
			}
			fill(img, roundRect{mx, my, m * .5, m * .5, m * .5}, ink) // neighbouring dots just touch
		}
	}
	last := float64(n - 4 - 7)
	for _, o := range [][2]float64{{4, 4}, {last, 4}, {4, last}} {
		ex, ey := (o[0]+3.5)*m, (o[1]+3.5)*m
		fill(img, roundRect{ex, ey, 3.5 * m, 3.5 * m, 1.7 * m}, ink)
		fill(img, roundRect{ex, ey, 2.5 * m, 2.5 * m, 1.1 * m}, image.White)
		fill(img, roundRect{ex, ey, 1.5 * m, 1.5 * m, .75 * m}, ink)
	}

	fill(img, roundRect{c, c, outer, outer, outer}, image.White)
	if logo != nil {
		drawLogo(img, c, c, inner, logo)
	} else {
		fill(img, roundRect{c, c, inner, inner, inner}, image.NewUniform(brand))
		drawLetter(img, c, c, inner, letter)
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// roundRect is an anti-aliased rounded-rectangle mask: centre, half width and height, corner
// radius. With the radius equal to the half sizes it is a circle.
type roundRect struct{ cx, cy, hw, hh, r float64 }

func (s roundRect) ColorModel() color.Model { return color.AlphaModel }
func (s roundRect) Bounds() image.Rectangle {
	return image.Rect(int(s.cx-s.hw)-1, int(s.cy-s.hh)-1, int(s.cx+s.hw)+2, int(s.cy+s.hh)+2)
}
func (s roundRect) At(x, y int) color.Color {
	// signed distance to the edge, negative inside
	qx := math.Abs(float64(x)+.5-s.cx) - (s.hw - s.r)
	qy := math.Abs(float64(y)+.5-s.cy) - (s.hh - s.r)
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - s.r
	return color.Alpha{A: uint8(math.Max(0, math.Min(1, .5-d)) * 255)}
}

func fill(dst draw.Image, s roundRect, src image.Image) {
	draw.DrawMask(dst, s.Bounds(), src, image.Point{}, s, s.Bounds().Min, draw.Over)
}

// drawLogo crops the logo to its centre square, scales it down to the badge and clips it round.
// Transparent logos sit on the white ring colour.
func drawLogo(dst *image.RGBA, cx, cy, r float64, logo image.Image) {
	lb := logo.Bounds()
	side := min(lb.Dx(), lb.Dy())
	crop := image.Rect(0, 0, side, side).Add(image.Pt(lb.Min.X+(lb.Dx()-side)/2, lb.Min.Y+(lb.Dy()-side)/2))
	d := int(math.Ceil(2 * r))
	small := image.NewRGBA(image.Rect(0, 0, d, d))
	draw.Draw(small, small.Bounds(), image.White, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(small, small.Bounds(), logo, crop, xdraw.Over, nil)

	m := roundRect{cx, cy, r, r, r}
	at := image.Pt(int(math.Floor(cx-r)), int(math.Floor(cy-r)))
	draw.DrawMask(dst, small.Bounds().Add(at), small, image.Point{}, m, at, draw.Over)
}

// drawLetter centres the letter's ink (not its advance box) in the badge.
func drawLetter(dst *image.RGBA, cx, cy, r float64, letter string) {
	if markFont == nil || letter == "" {
		return
	}
	face, err := opentype.NewFace(markFont, &opentype.FaceOptions{Size: r * 1.35, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return
	}
	defer face.Close()
	dr := &font.Drawer{Dst: dst, Src: image.White, Face: face}
	ink, _ := dr.BoundString(letter)
	w := float64(ink.Max.X-ink.Min.X) / 64
	h := float64(ink.Max.Y-ink.Min.Y) / 64
	x := cx - w/2 - float64(ink.Min.X)/64
	y := cy - h/2 - float64(ink.Min.Y)/64
	dr.Dot = fixed.Point26_6{X: fixed.Int26_6(x * 64), Y: fixed.Int26_6(y * 64)}
	dr.DrawString(letter)
}
