// Package ratelimit provides a small in-process fixed-window limiter for
// authentication endpoints. It is intentionally per-process; a multi-replica
// deployment should put a shared limiter in front of the API.
package ratelimit

import (
	"sync"
	"time"
)

type bucket struct {
	count    int
	expires  time.Time
	lastSeen time.Time
}

type Limiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
	now     func() time.Time
}

func New(limit int, window time.Duration) *Limiter {
	return &Limiter{
		limit:   limit,
		window:  window,
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// Allow consumes one attempt for key. When denied, retryAfter is the remaining
// window.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok || now.After(b.expires) {
		l.buckets[key] = &bucket{count: 1, expires: now.Add(l.window), lastSeen: now}
		l.compact(now)
		return true, 0
	}
	b.lastSeen = now
	if b.count >= l.limit {
		return false, b.expires.Sub(now)
	}
	b.count++
	return true, 0
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.buckets, key)
	l.mu.Unlock()
}

func (l *Limiter) compact(now time.Time) {
	if len(l.buckets) < 4096 {
		return
	}
	for key, b := range l.buckets {
		if now.After(b.expires) && now.Sub(b.lastSeen) > l.window {
			delete(l.buckets, key)
		}
	}
}
