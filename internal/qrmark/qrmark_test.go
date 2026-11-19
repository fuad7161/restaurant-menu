package qrmark

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"testing"
)

func solid(c color.RGBA, w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}

// The middle of the code shows the badge: the brand colour behind the letter, or the logo.
// (That the result still scans was checked with zxing-cpp at 120-640 px; see the commit message.)
func TestRenderBadge(t *testing.T) {
	brand := color.RGBA{0x1f, 0x6f, 0x4a, 0xff}
	red := color.RGBA{0xff, 0, 0, 0xff}
	for name, c := range map[string]struct {
		logo image.Image
		want color.RGBA
	}{
		"letter":    {nil, brand},
		"wide logo": {solid(red, 300, 200), red},
	} {
		b, err := Render("https://bosho.app/t/0123456789abcdef", 640, c.logo, "ব", brand)
		if err != nil {
			t.Fatal(name, err)
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(name, err)
		}
		mid := img.Bounds().Dx() / 2
		// inside the badge, below-left of the letter
		if got := color.RGBAModel.Convert(img.At(mid-40, mid+40)).(color.RGBA); got != c.want {
			t.Errorf("%s: badge pixel = %v, want %v", name, got, c.want)
		}
	}
}
