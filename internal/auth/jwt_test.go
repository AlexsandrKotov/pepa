package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func signToken(secret string, claims Claims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(secret))
	return signed
}

func TestMiddleware_DevMode_NoToken(t *testing.T) {
	r := gin.New()
	r.Use(Middleware("test-secret"))
	r.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID.String()})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	// No dev-mode bypass: a valid token is always required.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}
}

func TestMiddleware_ValidToken(t *testing.T) {
	secret := "test-secret"
	userID := uuid.New()
	tenantID := uuid.New()
	orgID := uuid.New()

	token := signToken(secret, Claims{
		UserID:         userID,
		TenantID:       tenantID,
		OrganizationID: orgID,
		Email:          "test@example.com",
		Roles:          []string{"admin"},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	r := gin.New()
	r.Use(Middleware(secret))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"user_id":   GetUserID(c).String(),
			"tenant_id": GetTenantID(c).String(),
			"org_id":    GetOrgID(c).String(),
			"roles":     GetRoles(c),
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !containsStr(w.Body.String(), userID.String()) {
		t.Errorf("expected user_id %s in response", userID)
	}
	if !containsStr(w.Body.String(), tenantID.String()) {
		t.Errorf("expected tenant_id %s in response", tenantID)
	}
}

func TestMiddleware_InvalidToken(t *testing.T) {
	r := gin.New()
	r.Use(Middleware("test-secret"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestMiddleware_WrongSecret(t *testing.T) {
	token := signToken("wrong-secret", Claims{
		TenantID: uuid.New(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	r := gin.New()
	r.Use(Middleware("correct-secret"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestMiddleware_ExpiredToken(t *testing.T) {
	token := signToken("secret", Claims{
		TenantID: uuid.New(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	})

	r := gin.New()
	r.Use(Middleware("secret"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestMiddleware_InvalidHeader(t *testing.T) {
	r := gin.New()
	r.Use(Middleware("secret"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestGetters_NoContext(t *testing.T) {
	c := &gin.Context{}

	if tid := GetTenantID(c); tid != (uuid.UUID{}) {
		t.Errorf("expected zero UUID, got %s", tid)
	}
	if oid := GetOrgID(c); oid != (uuid.UUID{}) {
		t.Errorf("expected zero UUID, got %s", oid)
	}
	if uid := GetUserID(c); uid != nil {
		t.Errorf("expected nil, got %v", uid)
	}
	if roles := GetRoles(c); roles != nil {
		t.Errorf("expected nil, got %v", roles)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMiddleware_ProdMode_NoToken(t *testing.T) {
	// Save and restore SERVER_ENV
	orig := os.Getenv("SERVER_ENV")
	defer os.Setenv("SERVER_ENV", orig)

	os.Setenv("SERVER_ENV", "production")

	r := gin.New()
	r.Use(Middleware("test-secret"))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	// No Authorization header
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 in production mode, got %d", w.Code)
	}
	if !containsStr(w.Body.String(), "missing authorization") {
		t.Errorf("expected 'missing authorization' error, got: %s", w.Body.String())
	}
}

func TestMiddleware_ProdMode_ValidToken(t *testing.T) {
	orig := os.Getenv("SERVER_ENV")
	defer os.Setenv("SERVER_ENV", orig)

	os.Setenv("SERVER_ENV", "production")

	secret := "prod-secret"
	token := signToken(secret, Claims{
		TenantID: uuid.New(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	r := gin.New()
	r.Use(Middleware(secret))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid token in production, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMiddleware_DevMode_Explicit(t *testing.T) {
	orig := os.Getenv("SERVER_ENV")
	defer os.Setenv("SERVER_ENV", orig)

	os.Setenv("SERVER_ENV", "development")

	r := gin.New()
	r.Use(Middleware("test-secret"))
	r.GET("/test", func(c *gin.Context) {
		tenantID := GetTenantID(c)
		c.JSON(200, gin.H{"tenant_id": tenantID.String()})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	// No Authorization header — should always fail (no dev-mode bypass).
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}
}

func TestIsPlatformAdmin(t *testing.T) {
	cases := []struct {
		name  string
		roles []string
		want  bool
	}{
		{"admin role", []string{"admin"}, true},
		{"super_admin role", []string{"super_admin"}, true},
		{"platform_admin role", []string{"platform_admin"}, true},
		{"platform admin with space", []string{"platform admin"}, true},
		{"case-insensitive", []string{"SUPER_ADMIN"}, true},
		{"among other roles", []string{"developer", "Admin"}, true},
		{"developer only", []string{"developer"}, false},
		{"viewer only", []string{"viewer"}, false},
		{"no roles", nil, false},
		{"empty roles", []string{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &gin.Context{}
			c.Set(CtxRoles, tc.roles)
			if got := IsPlatformAdmin(c); got != tc.want {
				t.Errorf("IsPlatformAdmin(roles=%v) = %v, want %v", tc.roles, got, tc.want)
			}
		})
	}
}
