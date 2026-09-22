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

func authTestRouter(deps Dependencies) *gin.Engine {
	router := gin.New()
	registerAuthRoutes(router, deps)
	return router
}

func TestSuperAdminResetRequiresPasswordChangeFlow(t *testing.T) {
	cfg := config.DefaultConfig()
	router := authTestRouter(Dependencies{Config: cfg})
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
