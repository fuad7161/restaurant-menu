// Package storage keeps dish photos in an S3-compatible store such as Backblaze B2.
package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
