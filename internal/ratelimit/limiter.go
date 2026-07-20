package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
	rate    float64
	burst   float64
	now     func() time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func New(perMinute, burst int) *Limiter {
	return &Limiter{
		buckets: make(map[string]bucket),
		rate:    float64(perMinute) / 60,
		burst:   float64(burst),
		now:     time.Now,
	}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	current, exists := l.buckets[key]
	if !exists {
		current = bucket{tokens: l.burst, last: now}
	}
	elapsed := now.Sub(current.last).Seconds()
	current.tokens = min(l.burst, current.tokens+elapsed*l.rate)
	current.last = now
	allowed := current.tokens >= 1
	if allowed {
		current.tokens--
	}
	l.buckets[key] = current
	if len(l.buckets) > 10_000 {
		l.cleanup(now)
	}
	return allowed
}

func (l *Limiter) cleanup(now time.Time) {
	maxIdle := time.Duration(l.burst/l.rate*2) * time.Second
	for key, value := range l.buckets {
		if now.Sub(value.last) > maxIdle {
			delete(l.buckets, key)
		}
	}
}

func min(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
