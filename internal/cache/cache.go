// Package cache keeps a shared "something changed" counter in Redis (Upstash).
//
// Every write to the app (any POST) bumps the counter. The screens that refresh every few
// seconds read it first: when it has not moved since the browser's last refresh, nothing on
// that screen can have changed, so the server answers 204 without querying PostgreSQL.
//
// Redis is only ever a shortcut. When it is not configured, slow or down, every call reports
// "unknown" and the app does the full database work exactly as it did before.
package cache

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const key = "bosho:changes"

// Versions is the shared change counter. A nil *Versions is valid and always reports "unknown".
type Versions struct {
	rdb *redis.Client
	// distrustUntil (unix nanoseconds) switches the shortcut off for a while after a failed bump,
	// so a lost bump cannot leave a screen showing old data.
	distrustUntil atomic.Int64
	distrustFor   time.Duration
}

// New connects to Redis. An empty URL returns nil: no Redis, no shortcut.
// distrustFor should be at least as long as the longest a refresh token stays valid.
func New(url string, distrustFor time.Duration) (*Versions, error) {
	if url == "" {
		return nil, nil
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	opt.DialTimeout = 2 * time.Second
	opt.ReadTimeout = 500 * time.Millisecond
	opt.WriteTimeout = 500 * time.Millisecond
	opt.MaxRetries = 1
	v := &Versions{rdb: redis.NewClient(opt), distrustFor: distrustFor}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := v.rdb.Ping(ctx).Err(); err != nil {
		v.rdb.Close()
		return nil, err
	}
	return v, nil
}

// Get returns the current counter. ok is false when it cannot be trusted right now.
func (v *Versions) Get(ctx context.Context) (n int64, ok bool) {
	if v == nil || time.Now().UnixNano() < v.distrustUntil.Load() {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	n, err := v.rdb.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, true // never bumped yet
	}
	if err != nil {
		return 0, false
	}
	return n, true
}

// Bump records that something changed. Call it after the change is saved in PostgreSQL.
func (v *Versions) Bump(ctx context.Context) {
	if v == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := v.rdb.Incr(ctx, key).Err(); err != nil {
		// Other screens would not learn about this change, so stop trusting the counter
		// until every refresh token handed out so far has expired.
		v.distrustUntil.Store(time.Now().Add(v.distrustFor).UnixNano())
		log.Println("redis: change counter not bumped, refresh shortcut paused:", err)
	}
}

// Close releases the connection.
func (v *Versions) Close() error {
	if v == nil {
		return nil
	}
	return v.rdb.Close()
}
