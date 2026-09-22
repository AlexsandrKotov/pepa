package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/config"
	"github.com/pepa/pepa/internal/database"
)

func authTestRequest(router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

func authTestRouter(t *testing.T, deps Dependencies) *gin.Engine {
	t.Helper()
	router := gin.New()
	if err := router.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registerAuthRoutes(router, deps))
	return router
}

func TestBootstrapActivationRateLimit(t *testing.T) {
	router := authTestRouter(t, Dependencies{Config: config.DefaultConfig()})
	const path = "/api/v1/auth/bootstrap/activate"
	for i := 0; i < 10; i++ {
		if response := authTestRequest(router, http.MethodPost, path, `{}`, ""); response.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d returned HTTP %d", i+1, response.Code)
		}
	}
	limited := authTestRequest(router, http.MethodPost, path, `{}`, "")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatal("activation requests were not rate limited before processing")
	}
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "198.51.100.2:1234"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatal("one client's limit affected another client")
	}
}

func TestBootstrapActivationBodyLimit(t *testing.T) {
	router := authTestRouter(t, Dependencies{Config: config.DefaultConfig()})
	body := `{"token":"test","new_password":"Aa1!` + strings.Repeat("x", 5000) + `"}`
	response := authTestRequest(router, http.MethodPost, "/api/v1/auth/bootstrap/activate", body, "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized activation body returned HTTP %d", response.Code)
	}
}

func TestBootstrapActivationConcurrencyLimit(t *testing.T) {
	router := gin.New()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan *httptest.ResponseRecorder, 2)
	router.POST("/activate", bootstrapActivationConcurrencyLimit(), func(c *gin.Context) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-t.Context().Done():
		}
		c.Status(http.StatusNoContent)
	})
	for i := 0; i < 2; i++ {
		go func() { done <- authTestRequest(router, http.MethodPost, "/activate", "", "") }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("activation did not reach handler")
		}
	}
	limited := authTestRequest(router, http.MethodPost, "/activate", "", "")
	close(release)
	for i := 0; i < 2; i++ {
		if response := <-done; response.Code != http.StatusNoContent {
			t.Fatalf("in-flight activation returned HTTP %d", response.Code)
		}
	}
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") == "" {
		t.Fatal("excess activation work was not rejected")
	}
	if response := authTestRequest(router, http.MethodPost, "/activate", "", ""); response.Code != http.StatusNoContent {
		t.Fatal("completed requests did not release activation slots")
	}
}

func TestSuperAdminResetRequiresPasswordChangeFlow(t *testing.T) {
	cfg := config.DefaultConfig()
	router := authTestRouter(t, Dependencies{Config: cfg})
	for _, tc := range []struct {
		name  string
		id    uuid.UUID
		roles []string
		want  int
	}{
		{"self", uuid.MustParse(database.SuperAdminUserID), []string{"admin"}, http.StatusForbidden},
		{"other admin", uuid.New(), []string{"admin"}, http.StatusForbidden},
		{"viewer", uuid.New(), []string{"viewer"}, http.StatusForbidden},
		{"anonymous", uuid.Nil, nil, http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var token string
			if tc.id != uuid.Nil {
				var err error
				token, err = auth.GenerateToken(cfg.Auth.JWTSecret, tc.id,
					uuid.MustParse(database.DefaultTenantID), uuid.MustParse(database.DefaultOrganizationID),
					"admin@local", tc.roles, 0, time.Hour)
				if err != nil {
					t.Fatal(err)
				}
			}
			response := authTestRequest(router, http.MethodPost,
				"/api/v1/auth/users/"+database.SuperAdminUserID+"/reset-password",
				`{"password":"Replacement-Password-2!"}`, token)
			if response.Code != tc.want {
				t.Fatalf("got HTTP %d, want %d", response.Code, tc.want)
			}
			if response.Header().Get("Set-Cookie") != "" {
				t.Fatal("rejected reset issued a session cookie")
			}
		})
	}
}
