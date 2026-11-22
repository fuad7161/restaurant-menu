package storage

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // decoders for uploaded photos
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Dish photos are stored in two sizes: a large one for the dish sheet and a small one for menu
// cards. Their names share a stem: "<stem>-l.jpg" and "<stem>-s.jpg". The large name is the one
// saved in the database. Photos uploaded before resizing existed have one plain name.
const (
	largeSuffix = "-l.jpg"
	smallSuffix = "-s.jpg"
	largeWidth  = 1200
	smallWidth  = 480
	jpegQuality = 80
)

// SmallName is the menu-card size of a stored photo, or the name itself for an older photo.
func SmallName(name string) string {
	if stem, ok := strings.CutSuffix(name, largeSuffix); ok {
		return stem + smallSuffix
	}
	return name
}

// HasSizes reports whether a stored photo has both sizes.
func HasSizes(name string) bool { return strings.HasSuffix(name, largeSuffix) }

// photoJPEG returns img as a JPEG at most maxWidth wide once turned upright for EXIF orientation o
// (never enlarged). It scales first and turns the small result, which is much faster.
// Transparent parts become white.
func photoJPEG(img image.Image, o, maxWidth int) ([]byte, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	upright := w
	if o >= 5 { // orientations 5 to 8 swap width and height
		upright = h
	}
	if upright > maxWidth {
		w, h = max(1, w*maxWidth/upright), max(1, h*maxWidth/upright)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, orient(dst, o), &jpeg.Options{Quality: jpegQuality})
	return buf.Bytes(), err
}

// jpegOrientation reads the EXIF orientation tag (1 to 8) from a JPEG; 1 (upright) if absent.
// Phone cameras record the rotation there instead of turning the pixels, and re-encoding drops
// EXIF, so without it photos would come out sideways.
func jpegOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if marker == 0xDA || size < 2 || i+2+size > len(data) { // start of image data, or broken
			return 1
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return exifOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func exifOrientation(t []byte) int {
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	ifd := int(bo.Uint32(t[4:]))
	if ifd+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[ifd:]))
	for k := 0; k < n; k++ {
		e := ifd + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 { // Orientation
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// orient returns img turned upright for an EXIF orientation value.
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if o >= 5 { // 5 to 8 swap width and height
		w, h = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = b.Dx()-1-x, y
			case 3:
				dx, dy = b.Dx()-1-x, b.Dy()-1-y
			case 4:
				dx, dy = x, b.Dy()-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = b.Dy()-1-y, x
			case 7:
				dx, dy = b.Dy()-1-y, b.Dx()-1-x
			case 8:
				dx, dy = y, b.Dx()-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
