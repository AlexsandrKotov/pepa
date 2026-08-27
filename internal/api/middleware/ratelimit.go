package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a simple token-bucket rate limiter per client IP.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*bucket
	rate     int           // tokens per interval
	interval time.Duration // refill interval
}

type bucket struct {
	tokens   int
	lastFill time.Time
}

// NewRateLimiter creates a rate limiter that allows `rate` requests per `interval` per IP.
func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*bucket),
		rate:     rate,
		interval: interval,
	}
	// Cleanup stale entries every 5 minutes
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, b := range rl.visitors {
			if now.Sub(b.lastFill) > 10*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) getBucket(ip string) *bucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.visitors[ip]
	if !ok {
		b = &bucket{tokens: rl.rate, lastFill: time.Now()}
		rl.visitors[ip] = b
		return b
	}

	// Refill tokens based on elapsed time
	elapsed := time.Since(b.lastFill)
	refills := int(elapsed / rl.interval)
	if refills > 0 {
		b.tokens += refills * rl.rate
		if b.tokens > rl.rate {
			b.tokens = rl.rate
		}
		b.lastFill = b.lastFill.Add(time.Duration(refills) * rl.interval)
	}

	return b
}

// Middleware returns a Gin middleware that enforces rate limiting.
func (rl *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		b := rl.getBucket(ip)

		rl.mu.Lock()
		if b.tokens > 0 {
			b.tokens--
			rl.mu.Unlock()
			c.Next()
			return
		}
		rl.mu.Unlock()

		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"error": "rate limit exceeded, please try again later",
		})
	}
}
