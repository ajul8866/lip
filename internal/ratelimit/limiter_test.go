package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := New(2, time.Minute)
	limiter.now = func() time.Time { return now }
	if ok, _ := limiter.Allow("k"); !ok {
		t.Fatal("first attempt should pass")
	}
	if ok, _ := limiter.Allow("k"); !ok {
		t.Fatal("second attempt should pass")
	}
	ok, retry := limiter.Allow("k")
	if ok || retry <= 0 {
		t.Fatalf("third attempt should be limited, retry=%v", retry)
	}
	limiter.Reset("k")
	if ok, _ := limiter.Allow("k"); !ok {
		t.Fatal("reset should clear the bucket")
	}
	now = now.Add(2 * time.Minute)
	if ok, _ := limiter.Allow("k"); !ok {
		t.Fatal("expired window should reset")
	}
}
