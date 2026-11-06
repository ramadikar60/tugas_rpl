package middleware

import (
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

type RateLimiter struct {
	visitors map[string]*bucket
	mu       sync.RWMutex
	rate     float64
	capacity float64
}

func NewRateLimiter(ratePerSec, burst float64) *RateLimiter {
	rl := &RateLimiter{
		visitors: map[string]*bucket{},
		rate:     ratePerSec,
		capacity: burst,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for k, b := range rl.visitors {
			b.mu.Lock()
			if now.Sub(b.lastRefill) > 10*time.Minute {
				delete(rl.visitors, k)
			}
			b.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.RLock()
	b, ok := rl.visitors[ip]
	rl.mu.RUnlock()

	if !ok {
		rl.mu.Lock()
		if b, ok = rl.visitors[ip]; !ok {
			b = &bucket{tokens: rl.capacity, lastRefill: time.Now()}
			rl.visitors[ip] = b
		}
		rl.mu.Unlock()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.capacity {
		b.tokens = rl.capacity
	}
	b.lastRefill = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"Too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func (rl *RateLimiter) MiddlewareHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, `{"error":"Too many requests"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	return ClientIP(r)
}
