//go:build integration

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/config"
	"github.com/pepa/pepa/internal/database"
	rbacengine "github.com/pepa/pepa/internal/rbac/engine"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/testenv"
)

const bootstrapTestPassword = "Bootstrap-Regression-1!" //nolint:gosec // G101: Disposable credentials for an isolated test database.

func bootstrapTestDeps(t *testing.T) (Dependencies, *testenv.PostgresContainer) {
	t.Helper()
	pg, err := testenv.StartPostgres(t.Context(), t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })
	resetBootstrapTestCache()
	t.Cleanup(resetBootstrapTestCache)
	engine := rbacengine.New(pg.DB().Pool)
	tenantID := uuid.MustParse(database.DefaultTenantID)
	if err := engine.SeedDefaultRoles(t.Context(), tenantID); err != nil {
		t.Fatal(err)
	}
	if err := engine.EnsureBasePermissions(t.Context(), tenantID); err != nil {
		t.Fatal(err)
	}
	return Dependencies{
		Config: config.DefaultConfig(), DB: pg.DB(), RBAC: engine,
		Repos: &Repositories{Auth: repository.NewAuthRepository(pg.DB().Pool)},
	}, pg
}

func resetBootstrapTestCache() {
	bootstrapComplete.Store(false)
	bootstrapLastCheck.Store(0)
	bootstrapStatusMu.Lock()
	bootstrapStatusCache = nil
	bootstrapStatusMu.Unlock()
}

func bootstrapTestExec(t *testing.T, db *database.DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.Pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func bootstrapTestActivate(router http.Handler, token, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"token": token, "new_password": password})
	return authTestRequest(router, http.MethodPost, "/api/v1/auth/bootstrap/activate", string(body), "")
}

func bootstrapTestLogin(t *testing.T, router http.Handler, password string, want int) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": "admin@local", "password": password})
	response := authTestRequest(router, http.MethodPost, "/api/v1/auth/login", string(body), "")
	if response.Code != want {
		t.Fatalf("password login returned HTTP %d, want %d", response.Code, want)
	}
	if want != http.StatusOK {
		return ""
	}
	var result struct {
		Token      string `json:"token"`
		MustChange bool   `json:"must_change_password"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || result.MustChange {
		t.Fatal("login must issue a session without requiring another password change")
	}
	return result.Token
}

func TestBootstrapPasswordLifecycle(t *testing.T) {
	deps, pg := bootstrapTestDeps(t)
	// Exercise real runtime permissions as well as the owner-run migrations.
	bootstrapTestExec(t, pg.DB(), `ALTER ROLE pepa_app PASSWORD 'bootstrap-integration-role'`)
	u, err := url.Parse(pg.ConnStr())
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("pepa_app", "bootstrap-integration-role")
	openRuntime := func() *database.DB {
		t.Helper()
		db, err := database.NewWithPin(u.String(), database.TenantPin{Mode: database.PinModePinned, TenantID: database.DefaultTenantID})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(db.Close)
		return db
	}
	useRuntime := func(db *database.DB) {
		deps.DB = db
		deps.Repos.Auth = repository.NewAuthRepository(db.Pool)
		deps.RBAC = rbacengine.New(db.Pool)
	}
	useRuntime(openRuntime())
	token, err := SeedBootstrapToken(deps)
	if err != nil || token == "" {
		t.Fatalf("seed bootstrap: %v", err)
	}
	if duplicate, err := SeedBootstrapToken(deps); err != nil || duplicate != "" {
		t.Fatalf("unexpected duplicate token: %v", err)
	}
	router := authTestRouter(t, deps)
	// A short session lets the test observe genuine JWT expiry without changing
	// the password or forging an expired cookie.
	deps.Config.Auth.SessionDuration = 2 * time.Second
	activated := bootstrapTestActivate(router, token, bootstrapTestPassword)
	if activated.Code != http.StatusOK {
		t.Fatalf("activation returned HTTP %d: %s", activated.Code, activated.Body)
	}
	var session struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(activated.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	claims, err := auth.ValidateJWT(session.Token, deps.Config.Auth.JWTSecret)
	if err != nil || claims.TokenVersion != 1 {
		t.Fatalf("activation must return the committed token version: %v", err)
	}
	cookies := activated.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("activation must issue an httpOnly cookie")
	}
	// Independent credential check: no activation cookie or token is sent.
	deps.Config.Auth.SessionDuration = time.Hour
	bootstrapTestLogin(t, router, bootstrapTestPassword, http.StatusOK)
	time.Sleep(time.Until(claims.ExpiresAt.Time) + 50*time.Millisecond)
	expired := authTestRequest(router, http.MethodPost, "/api/v1/auth/refresh", "", session.Token)
	if expired.Code != http.StatusUnauthorized {
		t.Fatal("expired sessions must remain invalid")
	}
	bootstrapTestLogin(t, router, bootstrapTestPassword, http.StatusOK)

	// Reproduce the auth-relevant restart sequence: a new runtime pool/router,
	// startup migrations, token seeding, and cleared process-local caches.
	deps.DB.Close()
	if err := pg.DB().RunMigrations(t.Context()); err != nil {
		t.Fatal(err)
	}
	resetBootstrapTestCache()
	useRuntime(openRuntime())
	bootstrapTestExec(t, pg.DB(), `UPDATE bootstrap_tokens SET expires_at = NOW() - INTERVAL '1 day'`)
	if replacement, err := SeedBootstrapToken(deps); err != nil || replacement != "" {
		t.Fatalf("completed setup must not generate a replacement token: %v", err)
	}
	router = authTestRouter(t, deps)
	oldSession := bootstrapTestLogin(t, router, bootstrapTestPassword, http.StatusOK)
	status := authTestRequest(router, http.MethodGet, "/api/v1/auth/bootstrap/status", "", "")
	var state struct {
		Needed     bool `json:"needed"`
		InProgress bool `json:"in_progress"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &state); err != nil || status.Code != http.StatusOK || state.Needed || state.InProgress {
		t.Fatal("completed bootstrap was lost after restart")
	}

	const replacementPassword = "Bootstrap-Replacement-2!" //nolint:gosec // G101: Disposable replacement credentials for the password lifecycle test.
	for _, body := range []string{
		`{"new_password":"Bootstrap-Replacement-2!"}`,
		`{"current_password":"wrong","new_password":"Bootstrap-Replacement-2!"}`,
	} {
		response := authTestRequest(router, http.MethodPost, "/api/v1/auth/me/reset-password", body, oldSession)
		if response.Code != http.StatusBadRequest && response.Code != http.StatusForbidden {
			t.Fatal("self-service password change accepted a missing or incorrect current password")
		}
	}
	body, _ := json.Marshal(map[string]string{"current_password": bootstrapTestPassword, "new_password": replacementPassword})
	changed := authTestRequest(router, http.MethodPost, "/api/v1/auth/me/reset-password", string(body), oldSession)
	if changed.Code != http.StatusOK {
		t.Fatalf("verified self-service change returned HTTP %d", changed.Code)
	}
	reset := authTestRequest(router, http.MethodPost, "/api/v1/auth/users/"+database.SuperAdminUserID+"/reset-password",
		`{"password":"Stale-Session-Attack-3!"}`, oldSession)
	if reset.Code != http.StatusForbidden {
		t.Fatal("a stale administrator session replaced the super admin password")
	}
	refresh := authTestRequest(router, http.MethodPost, "/api/v1/auth/refresh", "", oldSession)
	if refresh.Code != http.StatusUnauthorized {
		t.Fatal("revoked session must not refresh")
	}
	bootstrapTestLogin(t, router, bootstrapTestPassword, http.StatusUnauthorized)
	bootstrapTestLogin(t, router, replacementPassword, http.StatusOK)

	// Older versions could leave another unused token on a completed install.
	bootstrapTestExec(t, pg.DB(), `INSERT INTO bootstrap_tokens(token_hash, expires_at) VALUES ($1, NOW() + INTERVAL '1 hour')`, HashBootstrapToken("leftover-test-token"))
	for _, password := range []string{bootstrapTestPassword, "Aa1!" + strings.Repeat("x", 80)} {
		if response := bootstrapTestActivate(router, "leftover-test-token", password); response.Code != http.StatusForbidden {
			t.Fatalf("completed setup reached hashing or changed credentials: HTTP %d", response.Code)
		}
	}
	bootstrapTestLogin(t, router, replacementPassword, http.StatusOK)
}

func TestBootstrapActivationRollback(t *testing.T) {
	deps, _ := bootstrapTestDeps(t)
	router := authTestRouter(t, deps)
	for _, tc := range []struct {
		name, table, timing, body string
	}{
		{"token write error", "bootstrap_tokens", "BEFORE", "RAISE EXCEPTION 'injected token failure';"},
		{"password write error", "users", "BEFORE", "RAISE EXCEPTION 'injected password failure';"},
		{"zero rows updated", "users", "BEFORE", "RETURN NULL;"},
		{"wrong nonempty hash", "users", "BEFORE", "NEW.password_hash := OLD.password_hash; RETURN NEW;"},
		{"commit failure", "users", "AFTER", "RAISE EXCEPTION 'injected deferred failure';"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetBootstrapTestCache()
			bootstrapTestExec(t, deps.DB, `TRUNCATE bootstrap_tokens`)
			bootstrapTestExec(t, deps.DB, `UPDATE users SET password_hash='previous-nonempty-hash', must_change_password=true, token_version=7 WHERE id=$1`, uuid.MustParse(database.SuperAdminUserID))
			token, err := SeedBootstrapToken(deps)
			if err != nil || token == "" {
				t.Fatalf("seed bootstrap: %v", err)
			}
			bootstrapTestExec(t, deps.DB, `CREATE OR REPLACE FUNCTION bootstrap_test_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN `+tc.body+` END $$`)
			trigger := "CREATE TRIGGER bootstrap_test_fault " + tc.timing + " UPDATE ON " + tc.table + " FOR EACH ROW EXECUTE FUNCTION bootstrap_test_fault()"
			if tc.timing == "AFTER" {
				trigger = "CREATE CONSTRAINT TRIGGER bootstrap_test_fault AFTER UPDATE ON users DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION bootstrap_test_fault()"
			}
			bootstrapTestExec(t, deps.DB, trigger)
			response := bootstrapTestActivate(router, token, bootstrapTestPassword)
			bootstrapTestExec(t, deps.DB, "DROP TRIGGER bootstrap_test_fault ON "+tc.table)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("injected failure returned HTTP %d", response.Code)
			}
			if response.Header().Get("Set-Cookie") != "" || strings.Contains(response.Body.String(), `"token":`) {
				t.Fatal("failed activation issued a session")
			}
			if bootstrapComplete.Load() {
				t.Fatal("failed activation marked setup complete")
			}
			var used bool
			if err := deps.DB.Pool.QueryRow(t.Context(), `SELECT used_at IS NOT NULL FROM bootstrap_tokens WHERE token_hash=$1`, HashBootstrapToken(token)).Scan(&used); err != nil {
				t.Fatal(err)
			}
			user, err := deps.Repos.Auth.GetUserByID(t.Context(), uuid.MustParse(database.SuperAdminUserID))
			if err != nil {
				t.Fatal(err)
			}
			mustChange, err := deps.Repos.Auth.GetMustChangePassword(t.Context(), user.ID)
			if err != nil || used || user.PasswordHash != "previous-nonempty-hash" || user.TokenVersion != 7 || !mustChange {
				t.Fatal("failed activation did not roll back all credential and token changes")
			}
			if retried := bootstrapTestActivate(router, token, bootstrapTestPassword); retried.Code != http.StatusOK {
				t.Fatalf("same token was not retryable: HTTP %d", retried.Code)
			}
			bootstrapTestLogin(t, router, bootstrapTestPassword, http.StatusOK)
		})
	}
}

func TestBootstrapRejectsInvalidTokensAndHashFailure(t *testing.T) {
	deps, _ := bootstrapTestDeps(t)
	router := authTestRouter(t, deps)
	token, err := SeedBootstrapToken(deps)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token, password string
		want            int
	}{
		{"invalid", bootstrapTestPassword, http.StatusForbidden},
		// Bcrypt rejects overlong passwords. Invalid tokens must be rejected first.
		{"invalid", "Aa1!" + strings.Repeat("x", 80), http.StatusForbidden},
		{token, "weak", http.StatusBadRequest},
		{token, "Aa1!" + strings.Repeat("x", 80), http.StatusInternalServerError},
	} {
		response := bootstrapTestActivate(router, tc.token, tc.password)
		if response.Code != tc.want || response.Header().Get("Set-Cookie") != "" {
			t.Fatalf("invalid activation returned HTTP %d, want %d", response.Code, tc.want)
		}
	}
	var used bool
	if err := deps.DB.Pool.QueryRow(t.Context(), `SELECT used_at IS NOT NULL FROM bootstrap_tokens WHERE token_hash=$1`, HashBootstrapToken(token)).Scan(&used); err != nil || used {
		t.Fatal("invalid input consumed the bootstrap token")
	}
	bootstrapTestExec(t, deps.DB, `UPDATE bootstrap_tokens SET expires_at=NOW()-INTERVAL '1 hour'`)
	for _, password := range []string{bootstrapTestPassword, "Aa1!" + strings.Repeat("x", 80)} {
		if response := bootstrapTestActivate(router, token, password); response.Code != http.StatusForbidden {
			t.Fatalf("expired token reached hashing or was accepted: HTTP %d", response.Code)
		}
	}
	fresh, err := SeedBootstrapToken(deps)
	if err != nil || fresh == "" || fresh == token {
		t.Fatalf("pending setup could not replace an expired token: %v", err)
	}
	if response := bootstrapTestActivate(router, fresh, bootstrapTestPassword); response.Code != http.StatusOK {
		t.Fatalf("activation after invalid inputs failed: HTTP %d", response.Code)
	}
}

func TestBootstrapConcurrentActivation(t *testing.T) {
	deps, _ := bootstrapTestDeps(t)
	router := authTestRouter(t, deps)
	for _, distinctTokens := range []bool{false, true} {
		resetBootstrapTestCache()
		bootstrapTestExec(t, deps.DB, `TRUNCATE bootstrap_tokens`)
		bootstrapTestExec(t, deps.DB, `UPDATE users SET password_hash='', must_change_password=true, token_version=0 WHERE id=$1`, uuid.MustParse(database.SuperAdminUserID))
		tokens := []string{"concurrent-token-one", "concurrent-token-one"}
		if distinctTokens {
			tokens[1] = "concurrent-token-two"
		}
		for i, token := range tokens {
			if i == 0 || distinctTokens {
				bootstrapTestExec(t, deps.DB, `INSERT INTO bootstrap_tokens(token_hash, expires_at) VALUES ($1, NOW()+INTERVAL '1 hour')`, HashBootstrapToken(token))
			}
		}
		passwords := []string{bootstrapTestPassword, "Concurrent-Password-2!"}
		responses := make([]*httptest.ResponseRecorder, 2)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := range tokens {
			wg.Go(func() {
				<-start
				responses[i] = bootstrapTestActivate(router, tokens[i], passwords[i])
			})
		}
		close(start)
		wg.Wait()
		winner := -1
		for i, response := range responses {
			switch response.Code {
			case http.StatusOK:
				if winner != -1 {
					t.Fatal("more than one activation succeeded")
				}
				winner = i
			case http.StatusForbidden:
			default:
				t.Fatalf("concurrent activation returned HTTP %d", response.Code)
			}
		}
		if winner == -1 {
			t.Fatal("no activation succeeded")
		}
		bootstrapTestLogin(t, router, passwords[winner], http.StatusOK)
		bootstrapTestLogin(t, router, passwords[1-winner], http.StatusUnauthorized)
		user, err := deps.Repos.Auth.GetUserByID(t.Context(), uuid.MustParse(database.SuperAdminUserID))
		if err != nil || user.TokenVersion != 1 {
			t.Fatal("concurrent activation changed credentials more than once")
		}
	}
}
