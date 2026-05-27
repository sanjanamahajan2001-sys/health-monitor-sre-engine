package alert

import (
	"sync"
	"time"
)

type RateLimiter struct {
	rate   int
	burst  int
	tokens float64
	last   time.Time
	mu     sync.Mutex
}

func NewRateLimiter(rate int, burst int) *RateLimiter {
	if burst <= 0 {
		burst = rate
	}
	return &RateLimiter{
		rate:   rate,
		burst:  burst,
		tokens: float64(burst),
		last:   time.Now(),
	}
}

func (r *RateLimiter) Allow() bool {
	if r == nil || r.rate <= 0 {
		return true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(r.last).Seconds()
	r.tokens += elapsed * float64(r.rate)
	if r.tokens > float64(r.burst) {
		r.tokens = float64(r.burst)
	}
	r.last = now
	if r.tokens < 1 {
		return false
	}
	r.tokens -= 1
	return true
}
