package server

import (
	"context"
	"image"
	"image/color"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"bosho/internal/qrmark"
)

// QR images are drawn once per (link, logo, colour, name) and kept in memory: the tables page and
// the print sheet ask for every table at once, and the logo would otherwise be fetched from
// object storage for each one.
var qrCache = struct {
	sync.Mutex
	m map[string][]byte
}{m: map[string][]byte{}}

// qrImage is the PNG QR code for link with the restaurant's logo, or its first letter on the
// brand colour, in the middle.
func (a *App) qrImage(r *http.Request, rest Restaurant, link string) ([]byte, error) {
	key := strings.Join([]string{link, rest.Logo, rest.Color, rest.Name}, "\x00")
	qrCache.Lock()
	png, ok := qrCache.m[key]
	qrCache.Unlock()
	if ok {
		return png, nil
	}

	brand := color.RGBA{0x8f, 0x1d, 0x21, 0xff} // DefaultColor
	if c, ok := parseHex(rest.Color); ok {
		brand = color.RGBA{uint8(c.r), uint8(c.g), uint8(c.b), 0xff}
	}
	png, err := qrmark.Render(link, 640, a.qrLogo(r.Context(), rest.Logo), initial(rest.Name), brand)
	if err != nil {
		return nil, err
	}
	qrCache.Lock()
	if len(qrCache.m) > 500 { // old links, logos and colours pile up; start over now and then
		qrCache.m = map[string][]byte{}
	}
	qrCache.m[key] = png
	qrCache.Unlock()
	return png, nil
}

// qrLogo fetches and decodes the logo. Without one, or on any failure, it returns nil and the QR
// shows the letter instead.
func (a *App) qrLogo(ctx context.Context, name string) image.Image {
	if name == "" || a.photos == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	data, err := a.photos.Read(ctx, name)
	if err != nil {
		log.Println("qr logo:", err)
		return nil
	}
	img, err := qrmark.DecodeLogo(data)
	if err != nil {
		log.Println("qr logo:", err)
		return nil
	}
	return img
}
