package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// withOrientation returns a JPEG of w×h pixels (left half red, right half blue) carrying an EXIF
// orientation tag, as a phone camera writes it.
func withOrientation(t *testing.T, w, h, o int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if x >= w/2 {
				c = color.RGBA{0, 0, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, // big-endian TIFF header, first IFD at 8
		0, 1, // one entry
		0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, byte(o), 0, 0, // Orientation, SHORT, 1 value
		0, 0, 0, 0} // no next IFD
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := append([]byte{0xFF, 0xE1, byte((len(seg) + 2) >> 8), byte(len(seg) + 2)}, seg...)
	data := buf.Bytes()
	return append(append(append([]byte{}, data[:2]...), app1...), data[2:]...)
}

func TestOrientationTurnsPhotoUpright(t *testing.T) {
	// A 1600×800 landscape sensor image tagged "rotate 90° clockwise" (6) is a 800×1600 portrait photo.
	data := withOrientation(t, 1600, 800, 6)
	if o := jpegOrientation(data); o != 6 {
		t.Fatalf("orientation = %d, want 6", o)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out, err := photoJPEG(img, 6, smallWidth)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := got.Bounds(); b.Dx() != smallWidth || b.Dy() != smallWidth*2 {
		t.Fatalf("size = %dx%d, want %dx%d", b.Dx(), b.Dy(), smallWidth, smallWidth*2)
	}
	// Turned clockwise, the red left half is now on top.
	if r, _, bl, _ := got.At(smallWidth/2, 10).RGBA(); r < 0xc000 || bl > 0x4000 {
		t.Errorf("top is not red after turning")
	}
}

func TestNoEnlargingAndPlainNames(t *testing.T) {
	data := withOrientation(t, 300, 200, 1)
	img, _, _ := image.Decode(bytes.NewReader(data))
	out, _ := photoJPEG(img, 1, largeWidth)
	got, _ := jpeg.Decode(bytes.NewReader(out))
	if got.Bounds().Dx() != 300 {
		t.Errorf("a small photo was resized to %d wide", got.Bounds().Dx())
	}
	if SmallName("abc-l.jpg") != "abc-s.jpg" || SmallName("old.png") != "old.png" || HasSizes("old.png") {
		t.Error("photo names")
	}
	if jpegOrientation([]byte("not a jpeg")) != 1 {
		t.Error("orientation of a non-JPEG")
	}
}
