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

// Render returns a size×size PNG of a QR code for url with a round badge in the middle: logo
// when it is not nil, otherwise letter in white on brand.
func Render(url string, size int, logo image.Image, letter string, brand color.Color) ([]byte, error) {
	q, err := qrcode.New(url, qrcode.Highest)
	if err != nil {
		return nil, err
	}
	modules := len(q.Bitmap()) // includes the 4-module quiet zone on each side
	src := q.Image(size)
	img := image.NewRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Src)

	b := img.Bounds()
	symbol := float64(b.Dx()) * float64(modules-8) / float64(modules)
	cx, cy := float64(b.Min.X+b.Dx())/2, float64(b.Min.Y+b.Dy())/2
	outer := symbol * badgeShare / 2 // white ring, keeps the badge apart from the modules
	inner := outer * 0.86

	fillCircle(img, cx, cy, outer, image.White)
	if logo != nil {
		drawLogo(img, cx, cy, inner, logo)
	} else {
		fillCircle(img, cx, cy, inner, image.NewUniform(brand))
		drawLetter(img, cx, cy, inner, letter)
	}

	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// circle is an anti-aliased disc mask.
type circle struct{ cx, cy, r float64 }

func (c circle) ColorModel() color.Model { return color.AlphaModel }
func (c circle) Bounds() image.Rectangle {
	return image.Rect(int(c.cx-c.r)-1, int(c.cy-c.r)-1, int(c.cx+c.r)+2, int(c.cy+c.r)+2)
}
func (c circle) At(x, y int) color.Color {
	d := math.Hypot(float64(x)+0.5-c.cx, float64(y)+0.5-c.cy)
	a := math.Max(0, math.Min(1, c.r-d+0.5))
	return color.Alpha{A: uint8(a * 255)}
}

func fillCircle(dst draw.Image, cx, cy, r float64, src image.Image) {
	m := circle{cx, cy, r}
	draw.DrawMask(dst, m.Bounds(), src, image.Point{}, m, m.Bounds().Min, draw.Over)
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

	m := circle{cx, cy, r}
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
