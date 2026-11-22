// Package storage keeps dish photos in an S3-compatible store such as Backblaze B2.
package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MaxPhotoBytes is the largest photo accepted.
const MaxPhotoBytes = 5 << 20

var ErrNotImage = errors.New("file is not a JPEG, PNG or WebP image")

var extensions = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

type Photos struct {
	client *minio.Client
	bucket string
}

// New connects to the object store and creates the bucket when it is missing.
func New(endpoint, accessKey, secretKey, bucket, region string, useSSL bool) (*Photos, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, err
	}
	for attempt := 1; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var exists bool
		exists, err = client.BucketExists(ctx, bucket)
		if err == nil && !exists {
			err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		}
		cancel()
		if err == nil {
			return &Photos{client, bucket}, nil
		}
		if attempt == 30 {
			return nil, err
		}
		log.Printf("waiting for object storage (%v)", err)
		time.Sleep(2 * time.Second)
	}
}

// Save stores an uploaded image under a random name and returns that name.
// The content type is detected from the file itself, never trusted from the browser.
func (p *Photos) Save(ctx context.Context, r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxPhotoBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > MaxPhotoBytes {
		return "", errors.New("photo is larger than 5 MB")
	}
	contentType := http.DetectContentType(data)
	ext, ok := extensions[contentType]
	if !ok {
		return "", ErrNotImage
	}
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	name := hex.EncodeToString(random) + ext
	_, err = p.client.PutObject(ctx, p.bucket, name, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	return name, err
}

// SaveDish stores an uploaded dish photo in two sizes (see resize.go) and returns the large name.
func (p *Photos) SaveDish(ctx context.Context, r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxPhotoBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > MaxPhotoBytes {
		return "", errors.New("photo is larger than 5 MB")
	}
	if _, ok := extensions[http.DetectContentType(data)]; !ok {
		return "", ErrNotImage
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", ErrNotImage
	}
	o := jpegOrientation(data)
	large, err := photoJPEG(img, o, largeWidth)
	if err != nil {
		return "", err
	}
	small, err := photoJPEG(img, o, smallWidth)
	if err != nil {
		return "", err
	}
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	stem := hex.EncodeToString(random)
	for name, b := range map[string][]byte{stem + smallSuffix: small, stem + largeSuffix: large} {
		if _, err := p.client.PutObject(ctx, p.bucket, name, bytes.NewReader(b), int64(len(b)),
			minio.PutObjectOptions{ContentType: "image/jpeg"}); err != nil {
			return "", err
		}
	}
	return stem + largeSuffix, nil
}

// Serve streams a stored photo to the browser. Names are random and never reused, so browsers may cache for a long time.
func (p *Photos) Serve(w http.ResponseWriter, r *http.Request, name string) {
	obj, err := p.client.GetObject(r.Context(), p.bucket, name, minio.GetObjectOptions{})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer obj.Close()
	info, err := obj.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", info.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, "", info.LastModified, obj)
}

// Read returns a stored photo's bytes, for server-side use such as drawing the logo into QR codes.
func (p *Photos) Read(ctx context.Context, name string) ([]byte, error) {
	obj, err := p.client.GetObject(ctx, p.bucket, name, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	return io.ReadAll(io.LimitReader(obj, MaxPhotoBytes+1))
}
