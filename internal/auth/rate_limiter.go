package auth

import (
	"sync"
	"time"
)

// LoginRateLimiter provides brute-force protection for login attempts.
// It tracks failed attempts per key (email or IP) and locks accounts
// after too many failures within a time window.
type LoginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
	maxFails int
	window   time.Duration
	lockout  time.Duration
}

type loginAttempt struct {
	count     int
	lastFail  time.Time
	lockedUntil time.Time
}

// NewLoginRateLimiter creates a rate limiter with the given parameters.
// maxFails: number of failed attempts before lockout.
// window: time window in which failures are counted.
// lockout: duration of the lockout after maxFails.
func NewLoginRateLimiter(maxFails int, window, lockout time.Duration) *LoginRateLimiter {
	r := &LoginRateLimiter{
		attempts: make(map[string]*loginAttempt),
		maxFails: maxFails,
		window:   window,
		lockout:  lockout,
	}
	// Start cleanup goroutine to prevent memory leaks.
	go r.cleanup()
	return r
}

// Allow checks whether the key is currently allowed to attempt login.
// Returns (allowed bool, retryAfter time.Duration).
func (r *LoginRateLimiter) Allow(key string) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.attempts[key]
	if !ok {
		return true, 0
	}

	// If currently locked, return remaining lockout time.
	if time.Now().Before(a.lockedUntil) {
		return false, a.lockedUntil.Sub(time.Now())
	}

	// If the window has passed since last failure, reset counter.
	if time.Since(a.lastFail) > r.window {
		delete(r.attempts, key)
		return true, 0
	}

	return true, 0
}

// RecordFailure records a failed login attempt for the key.
// If maxFails is reached, the key is locked out.
func (r *LoginRateLimiter) RecordFailure(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.attempts[key]
	if !ok {
		a = &loginAttempt{}
		r.attempts[key] = a
	}

	// If window expired, reset.
	if time.Since(a.lastFail) > r.window {
		a.count = 0
	}

	a.count++
	a.lastFail = time.Now()

	if a.count >= r.maxFails {
		a.lockedUntil = time.Now().Add(r.lockout)
	}
}

// RecordSuccess clears the failure counter for the key on successful login.
func (r *LoginRateLimiter) RecordSuccess(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attempts, key)
}

// cleanup periodically removes expired entries to prevent memory leaks.
func (r *LoginRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		r.mu.Lock()
		now := time.Now()
		for key, a := range r.attempts {
			// Remove entries that are no longer locked and whose window has expired.
			if now.After(a.lockedUntil) && now.Sub(a.lastFail) > r.window {
				delete(r.attempts, key)
			}
		}
		r.mu.Unlock()
	}
}
