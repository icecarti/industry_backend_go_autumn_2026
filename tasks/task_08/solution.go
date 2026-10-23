package main

import (
	"sync"
	"time"
)

type Clock interface{ Now() time.Time }
type Limiter struct {
	mu     sync.Mutex
	clock  Clock
	rate   float64
	burst  int
	tokens float64
	last   time.Time
}

func NewLimiter(clock Clock, ratePerSec float64, burst int) *Limiter {
	limiter := &Limiter{
		clock:  clock,
		rate:   ratePerSec,
		burst:  burst,
	}

	if burst > 0 {
		limiter.tokens = float64(burst)
	}

	if clock != nil {
		limiter.last = clock.Now()
	}
	return limiter
}
func (l *Limiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if n <= 0 || n > l.burst || l.burst <= 0 || l.clock == nil {
		return false
	}

	now := l.clock.Now()
	if now.After(l.last) {
		delta := now.Sub(l.last).Seconds()
		if l.rate > 0 {
			l.tokens += delta * l.rate
			if l.tokens > float64(l.burst) {
				l.tokens = float64(l.burst)
			}
		}
		l.last = now
	}

	if l.tokens < float64(n) {
		return false
	}
	l.tokens -= float64(n)
	return true
}
