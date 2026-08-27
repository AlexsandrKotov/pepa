package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10, time.Second)
	if rl == nil {
		t.Fatal("NewRateLimiter returned nil")
	}
	if rl.rate != 10 {
		t.Errorf("rate = %d, want 10", rl.rate)
	}
	if rl.interval != time.Second {
		t.Errorf("interval = %v, want 1s", rl.interval)
	}
}

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(5, time.Second)

	r := gin.New()
	r.Use(rl.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Make 5 requests (should all succeed)
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute) // long interval to prevent refill

	r := gin.New()
	r.Use(rl.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Use up all tokens
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// Next request should be rate limited
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)

	r := gin.New()
	r.Use(rl.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// IP 1 uses its token
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "1.1.1.1:1234"
	r.ServeHTTP(w1, req1)
	if w1.Code != 200 {
		t.Fatalf("IP1 first request: expected 200, got %d", w1.Code)
	}

	// IP 2 should also have its own token
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "2.2.2.2:1234"
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("IP2 first request: expected 200, got %d", w2.Code)
	}

	// IP 1 should be blocked now
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/test", nil)
	req3.RemoteAddr = "1.1.1.1:1234"
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 second request: expected 429, got %d", w3.Code)
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	rl := NewRateLimiter(2, 50*time.Millisecond)

	r := gin.New()
	r.Use(rl.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// Use all tokens from 1.1.1.1
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.1.1.1:1234"
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// Wait for refill
	time.Sleep(100 * time.Millisecond)

	// Should be allowed again
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.1.1.1:1234"
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("expected 200 after refill, got %d", w.Code)
	}
}

func TestGetBucket_CreatesNewBucket(t *testing.T) {
	rl := NewRateLimiter(5, time.Second)
	b := rl.getBucket("10.0.0.1")
	if b == nil {
		t.Fatal("expected non-nil bucket")
	}
	if b.tokens != 5 {
		t.Errorf("expected 5 tokens, got %d", b.tokens)
	}
}

func TestGetBucket_ReturnsSameBucket(t *testing.T) {
	rl := NewRateLimiter(5, time.Second)
	b1 := rl.getBucket("10.0.0.1")
	b2 := rl.getBucket("10.0.0.1")
	if b1 != b2 {
		t.Error("expected same bucket pointer for same IP")
	}
}

func TestGetBucket_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(100, time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			b := rl.getBucket(ip)
			if b == nil {
				t.Errorf("nil bucket for %s", ip)
			}
		}("10.0.0." + string(rune('1'+i%10)))
	}
	wg.Wait()
}

func TestMiddleware_RateLimitResponse(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)

	r := gin.New()
	r.Use(rl.Middleware())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	// First request OK
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "5.5.5.5:1234"
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Second request rate limited
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "5.5.5.5:1234"
	r.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("expected 429, got %d", w.Code)
	}

	// Check response body contains error message
	body := w.Body.String()
	if !containsStr(body, "rate limit exceeded") {
		t.Errorf("expected rate limit error message, got: %s", body)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
