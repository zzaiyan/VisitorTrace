package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterRefillsTokens(t *testing.T) {
	limiter := New(60, 2)
	now := time.Date(2026, time.July, 21, 0, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }
	if !limiter.Allow("key") || !limiter.Allow("key") || limiter.Allow("key") {
		t.Fatal("burst token behavior is incorrect")
	}
	now = now.Add(time.Second)
	if !limiter.Allow("key") {
		t.Fatal("limiter did not refill one token")
	}
}
