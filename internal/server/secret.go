package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
)

// passwordCipher makes the AES-GCM cipher for staff passwords the owner can look up. The key comes
// from PASSWORD_KEY, or else from the database, where the first start stores a random one.
func (a *App) passwordCipher() (cipher.AEAD, error) {
	key64 := a.cfg.PasswordKey
	if key64 == "" {
		fresh := make([]byte, 32)
		rand.Read(fresh)
		a.db.Exec(`INSERT INTO platform_settings(key,value) VALUES('password_key',?) ON CONFLICT (key) DO NOTHING`, base64.StdEncoding.EncodeToString(fresh))
		if err := a.db.QueryRow(`SELECT value FROM platform_settings WHERE key='password_key'`).Scan(&key64); err != nil {
			return nil, fmt.Errorf("password key: %w", err)
		}
		slog.Info("staff password key read from the database (set PASSWORD_KEY to keep it outside)")
	}
	key, err := base64.StdEncoding.DecodeString(key64)
	if err != nil || len(key) != 32 {
		return nil, errors.New("PASSWORD_KEY must be 32 bytes in base64 (openssl rand -base64 32)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// sealPassword encrypts a staff password so the owner can see it again later.
func (a *App) sealPassword(pass string) string {
	nonce := make([]byte, a.pwKey.NonceSize())
	rand.Read(nonce)
	return base64.StdEncoding.EncodeToString(a.pwKey.Seal(nonce, nonce, []byte(pass), nil))
}

// openPassword decrypts a sealed staff password; "" when there is none or it can't be read.
func (a *App) openPassword(sealed string) string {
	b, err := base64.StdEncoding.DecodeString(sealed)
	n := a.pwKey.NonceSize()
	if sealed == "" || err != nil || len(b) < n {
		return ""
	}
	plain, err := a.pwKey.Open(nil, b[:n], b[n:], nil)
	if err != nil {
		return ""
	}
	return string(plain)
}

// tokenHash is how email link tokens are stored: a leaked table can't be turned into links.
func tokenHash(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}
