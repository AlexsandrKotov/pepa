package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // #nosec G108 //nolint:gosec // pprof is intentionally enabled for diagnostics
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pepa/pepa/internal/api/rest"
	"github.com/pepa/pepa/internal/bootstrap"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/gitops"
	rbacengine "github.com/pepa/pepa/internal/rbac/engine"
	"github.com/pepa/pepa/internal/security"
	"github.com/pepa/pepa/internal/service"
	"github.com/pepa/pepa/pkg/models"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	// Support -migrate-only flag: run migrations then exit.
	migrateOnly := false
	for _, arg := range os.Args[1:] {
		if arg == "-migrate-only" || arg == "--migrate-only" {
			migrateOnly = true
			break
		}
	}

	slog.Info("PEPA API server starting", "version", version, "build_time", buildTime)

	// Create a root context that is cancelled on SIGINT/SIGTERM.
	// All background goroutines (plugin discovery, doc seeding, RAG watcher)
	// derive from this context so they exit cleanly on shutdown.
	rootCtx, rootCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer rootCancel()

	// Bootstrap all shared components
	comp, err := bootstrap.Bootstrap(rootCtx)
	if err != nil {
		slog.Error("bootstrap failed", "error", err)
		os.Exit(1)
	}

	// Migrations were already applied inside bootstrap, on the table-owner
	// connection. Re-running them here would use the runtime pool instead, whose
	// app role is not allowed to create objects in the schema.

	// If -migrate-only, exit successfully after migrations.
	if migrateOnly {
		slog.Info("migrate-only mode — exiting after successful migration")
		if comp.DB.Pool != nil {
			comp.DB.Pool.Close()
		}
		os.Exit(0)
	}

	// Migrate plain text credentials to encrypted storage
	// (AutoRegisterPlugins runs asynchronously inside Bootstrap after plugin discovery)
	comp.MigrateEncryptCredentials()

	// Seed default public Helm repositories
	comp.SeedDefaultHelmRepos()

	// Start event bus
	comp.StartEventBus()

	// Write system startup audit event
	writeSystemAuditEvent(comp, "startup", "system", map[string]interface{}{
		"version":    version,
		"build_time": buildTime,
	})

	// Initialize RBAC engine and seed default roles
	rbacEngine := rbacengine.New(comp.DB.Pool)
	defaultTenantID := uuid.MustParse(database.DefaultTenantID)
	if err := rbacEngine.SeedDefaultRoles(context.Background(), defaultTenantID); err != nil {
		slog.Warn("failed to seed default roles", "error", err)
	} else {
		slog.Info("RBAC default roles seeded")
	}
	// Ensure all system roles have correct base permissions (idempotent).
	if err := rbacEngine.EnsureBasePermissions(context.Background(), defaultTenantID); err != nil {
		slog.Warn("failed to ensure base permissions", "error", err)
	} else {
		slog.Info("RBAC base permissions verified")
	}

	// Report what row-level security actually covers.
	reportRLSCoverage(comp)

	// Seed bootstrap token for first-run setup
	bootstrapDeps := rest.Dependencies{
		Config: comp.Config,
		DB:     comp.DB,
		Repos:  &rest.Repositories{},
		RBAC:   rbacEngine,
	}
	if rawToken, err := rest.SeedBootstrapToken(bootstrapDeps); err != nil {
		slog.Warn("failed to seed bootstrap token", "error", err)
	} else if rawToken != "" {
		fmt.Fprintln(os.Stderr, "============================================================")
		fmt.Fprintln(os.Stderr, "  FIRST-RUN SETUP — Bootstrap Token Generated")
		fmt.Fprintln(os.Stderr, "============================================================")

		// Always write token to a file — never print to logs.
		// Logs are often captured by aggregation systems (ELK, Loki, CloudWatch)
		// where secrets can linger in indexes and be exposed to wider audiences.
		tokenPath := os.Getenv("BOOTSTRAP_TOKEN_PATH")
		if tokenPath == "" {
			tokenPath = "/var/run/pepa/bootstrap_token.txt" //nolint:gosec // #nosec // G101: not a credential, just a default file path
		}
		if writeErr := os.MkdirAll(filepath.Dir(tokenPath), 0700); writeErr != nil { //nolint:gosec // #nosec // G703: tokenPath is admin-controlled (env var or hardcoded default)
			slog.Warn("failed to create token directory", "error", writeErr)
		}
		if writeErr := os.WriteFile(tokenPath, []byte(rawToken+"\n"), 0600); writeErr != nil { //nolint:gosec // #nosec // G703: tokenPath is admin-controlled (env var or hardcoded default)
			slog.Warn("failed to write bootstrap token file", "error", writeErr)
			fmt.Fprintln(os.Stderr, "  ERROR: could not write token file — check BOOTSTRAP_TOKEN_PATH")
		} else {
			fmt.Fprintf(os.Stderr, "  Token written to: %s\n", tokenPath)
			fmt.Fprintln(os.Stderr, "  Read it now, it will not be shown again.")
		}
		fmt.Fprintln(os.Stderr, "  This token expires in 1 hour.")
		fmt.Fprintln(os.Stderr, "  Use it to log in and create the first users.")
		fmt.Fprintln(os.Stderr, "  After login, you will be prompted to change your password.")
		fmt.Fprintln(os.Stderr, "============================================================")
	}

	checkAdminPasswordHealth(rootCtx, comp.DB.Pool, slog.Default())

	// AI manager is initialized by bootstrap; available as comp.AIManager

	// Build dependencies struct
	deps := rest.Dependencies{
		Config: comp.Config,
		DB:     comp.DB,
		Repos: &rest.Repositories{
			Entity:              comp.EntityRepo,
			Workflow:            comp.WorkflowRepo,
			Plugin:              comp.PluginRepo,
			Scorecard:           comp.ScorecardRepo,
			Audit:               comp.AuditRepo,
			Cluster:             comp.ClusterRepo,
			Deployment:          comp.DeploymentRepo,
			Jira:                comp.JiraRepo,
			Connection:          comp.ConnectionRepo,
			Service:             comp.ServiceRepo,
			Settings:            comp.SettingsRepo,
			Environment:         comp.EnvironmentRepo,
			EnvVariable:         comp.EnvVariableRepo,
			DockerHost:          comp.DockerHostRepo,
			Helm:                comp.HelmRepo,
			Registry:            comp.RegistryRepo,
			PipelineSource:      comp.PipelineSourceRepo,
			PipelinePreset:      comp.PipelinePresetRepo,
			PipelineRun:         comp.PipelineRunRepo,
			Vault:               comp.VaultRepo,
			VaultConfig:         comp.VaultConfigRepo,
			Auth:                comp.AuthRepo,
			TeamWorkflow:        comp.TeamWorkflowRepo,
			GitopsRepo:          comp.GitopsRepo,
			UserCredential:      comp.UserCredentialRepo,
			CredentialShare:     comp.CredentialShareRepo,
			Organization:        comp.OrganizationRepo,
			RAG:                 comp.RAGRepo,
			SSHHost:             comp.SSHHostRepo,
			SSHHostGroup:        comp.SSHHostGroupRepo,
			PluginActivity:      comp.PluginActivityRepo,
			SecurityScan:        comp.SecurityScanRepo,
			ScanIgnore:          comp.ScanIgnoreRepo,
			DevOps:              comp.DevOpsRepo,
			NotificationRule:    comp.NotificationRuleRepo,
			NotificationLog:     comp.NotificationLogRepo,
			GitOpsBinding:       comp.GitOpsBindingRepo,
			DriftSchedule:       comp.DriftScheduleRepo,
			EnvironmentOverview: comp.EnvironmentOverviewRepo,
			SelfService:         comp.SelfServiceRepo,
			AutoDeployRule:      comp.AutoDeployRuleRepo,
		},
		Services: &rest.Services{
			Deployment: func() *service.DeploymentService {
				svc := service.NewDeploymentService(
					comp.ClusterRepo,
					comp.DeploymentRepo,
					comp.HelmRepo,
				)
				// Wire up deployment event recorder for timeline
				if comp.DB != nil {
					svc.SetEventRecorder(func(deploymentID uuid.UUID, eventType, message string) {
						_, _ = comp.DB.Pool.Exec(context.Background(),
							`INSERT INTO deployment_events (deployment_id, event_type, message) VALUES ($1, $2, $3)`,
							deploymentID, eventType, message)
					})
				}
				return svc
			}(),
			ServiceDeployment: service.NewServiceDeploymentService(
				comp.ClusterRepo,
				comp.ServiceRepo,
				comp.HelmRepo,
			),
			Connection: func() *service.ConnectionService {
				svc := service.NewConnectionService()
				if comp.DB != nil {
					svc.SetBuiltinVaultCheck(func(ctx context.Context) error {
						var ready bool
						return comp.DB.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM vault_secrets LIMIT 1)").Scan(&ready)
					})
				}
				return svc
			}(),
			NotificationDispatcher: func() *service.NotificationDispatcher {
				d := service.NewNotificationDispatcher(
					comp.NotificationRuleRepo,
					comp.NotificationLogRepo,
					comp.ConnectionRepo,
					comp.PluginMgr,
				)
				if comp.EventBus != nil {
					d.RegisterHandlers(comp.EventBus)
				}
				return d
			}(),
			EntitySync:    service.NewEntitySyncService(comp.EntityRepo),
			ScorecardEval: service.NewScorecardEvalService(comp.ScorecardRepo, comp.EntityRepo),
		},
		PluginMgr:        comp.PluginMgr,
		ProviderRegistry: comp.ProviderRegistry,
		PipelineRegistry: comp.PipelineRegistry,
		EventBus:         comp.EventBus,
		JobQueue:         comp.JobQueue,
		AIManager:        comp.AIManager,
		IngestionEngine:  comp.IngestionEngine,
		RAGPipeline:      comp.RAGPipeline,
		RiskScorer:       comp.RiskScorer,
		DocGenerator:     comp.DocGenerator,
		CostAdvisor:      comp.CostAdvisor,
		StaleDetector:    comp.StaleDetector,
		WorkflowBuilder:  comp.WorkflowBuilder,
		RBAC:             rbacEngine,
		Storage:          comp.Storage,
		Scanner:          security.NewScanner(comp.PluginMgr, comp.SecurityScanRepo, comp.ConnectionRepo, comp.RegistryRepo, comp.ScanIgnoreRepo),
		Version:          version,
		BuildTime:        buildTime,
	}

	// Initialize drift detection scheduler
	driftDetectFn := rest.BuildDriftDetectionFunc(deps)
	driftScheduler := gitops.NewDriftScheduler(comp.DriftScheduleRepo, driftDetectFn)
	driftScheduler.SetAlertFunc(rest.BuildDriftAlertFunc(deps))
	deps.DriftScheduler = driftScheduler

	// Start drift scheduler in background
	go driftScheduler.Start(rootCtx)
	slog.Info("drift detection scheduler started")

	// Initialize security scan scheduler
	scanScheduler := security.NewScheduler(comp.SecurityScanRepo, deps.Scanner)
	deps.ScanScheduler = scanScheduler

	// Start scan scheduler in background
	go scanScheduler.Start(rootCtx)
	slog.Info("security scan scheduler started")

	// Auto-start Trivy DB manager if the Trivy plugin is already enabled.
	// This ensures the vulnerability databases are downloaded/refreshed on
	// server restart without requiring the user to re-enable the plugin.
	if trivyPlugin, err := comp.PluginRepo.GetByName(rootCtx, "trivy"); err == nil && trivyPlugin.Enabled && deps.Scanner != nil {
		slog.Info("Trivy plugin is enabled, starting DB manager on startup")
		deps.Scanner.StartDBManager(rootCtx)
	}

	// Initialize HTTP router
	router, shutdownRouter := rest.NewRouter(deps)

	// Start pprof server on separate port (if enabled)
	if comp.Config.Server.PprofEnabled {
		go func() {
			pprofAddr := fmt.Sprintf("%s:%d", comp.Config.Server.Host, comp.Config.Server.PprofPort)
			slog.Info("pprof server starting", "addr", pprofAddr)
			pprofMux := http.NewServeMux()
			pprofMux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
			pprofMux.HandleFunc("/debug/pprof/cmdline", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.DefaultServeMux.ServeHTTP(w, r)
			}))
			pprofMux.HandleFunc("/debug/pprof/profile", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.DefaultServeMux.ServeHTTP(w, r)
			}))
			pprofMux.HandleFunc("/debug/pprof/symbol", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.DefaultServeMux.ServeHTTP(w, r)
			}))
			pprofMux.HandleFunc("/debug/pprof/trace", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.DefaultServeMux.ServeHTTP(w, r)
			}))
			pprofServer := &http.Server{
				Addr:              pprofAddr,
				Handler:           pprofMux,
				ReadHeaderTimeout: 10 * time.Second,
			}
			if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("pprof server failed", "error", err)
			}
		}()
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%s", comp.Config.Server.Host, comp.Config.Server.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 15 * time.Minute, // extended for LLM calls and SSE streaming
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown — wait for the root context to be cancelled
	// (SIGINT/SIGTERM) instead of using a separate signal channel.
	go func() {
		slog.Info("PEPA API listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-rootCtx.Done()
	slog.Info("shutting down gracefully")

	// Stop drift detection scheduler
	driftScheduler.Stop()

	// Stop security scan scheduler
	scanScheduler.Stop()

	// Write shutdown audit event
	writeSystemAuditEvent(comp, "shutdown", "system", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	comp.Shutdown(ctx)
	shutdownRouter()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}

	slog.Info("PEPA stopped")
}

// checkAdminPasswordHealth reports only observed state, never an inferred cause
// of credential loss. Database failures are distinct from missing credentials.
func checkAdminPasswordHealth(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var hasUsedToken, hasPassword bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM bootstrap_tokens WHERE used_at IS NOT NULL),
		       EXISTS(SELECT 1 FROM users WHERE id = $1 AND COALESCE(password_hash, '') != '')
	`, uuid.MustParse(database.SuperAdminUserID)).Scan(&hasUsedToken, &hasPassword)
	if err != nil {
		logger.Warn("super admin password health check failed", "error", err)
		return
	}
	if hasUsedToken && !hasPassword {
		logger.Error("super admin password is unavailable after bootstrap; password login will fail",
			"admin_id", database.SuperAdminUserID)
	}
}

// writeSystemAuditEvent writes a system-level audit event (no HTTP context).
func writeSystemAuditEvent(comp *bootstrap.Components, action, entityType string, data map[string]interface{}) {
	if comp.AuditRepo == nil {
		return
	}
	tenantID := uuid.MustParse(database.DefaultTenantID)
	entry := &models.AuditLog{
		TenantID:   tenantID,
		Action:     action,
		EntityType: entityType,
	}
	if data != nil {
		raw, _ := json.Marshal(data)
		entry.NewValues = raw
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := comp.AuditRepo.Create(ctx, entry); err != nil {
		slog.Warn("failed to write system audit event", "action", action, "error", err)
	}
}

// reportRLSCoverage logs how much of the schema row-level security actually
// protects, once, at startup.
//
// In PEPA the repositories' explicit tenant_id filters are the isolation
// control; RLS is defence-in-depth on top of them. That layer is easy to believe
// in while it silently applies to nothing: the application role typically owns
// the tables (owners skip policies unless they are FORCED), and a policy reading
// the retired 'app.current_tenant' setting never matches the GUC the code sets.
// Saying it out loud at startup is what keeps "RLS is on" from being an
// assumption nobody checked.
func reportRLSCoverage(comp *bootstrap.Components) {
	if comp.DB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	report, err := comp.DB.InspectRLS(ctx)
	if err != nil {
		slog.Warn("RLS self-check failed", "error", err)
		return
	}

	attrs := []any{
		"role", report.RoleName,
		"policies", report.Policies,
		"rls_enabled_tables", report.EnabledTables,
		"rls_forced_tables", report.ForcedTables,
		"tenant_tables_without_policy", report.TablesWithTenantColumnWithoutPolicy,
	}
	if report.LegacyGUCPolicies > 0 {
		slog.Error("RLS policies still read the retired app.current_tenant setting and therefore never match",
			append(attrs, "legacy_guc_policies", report.LegacyGUCPolicies, "expected_guc", database.TenantGUC)...)
	}
	if report.RoleBypassesRLS {
		slog.Warn("row-level security does NOT apply to the application role; tenant isolation relies on repository query filters",
			append(attrs, "bypasses_rls", true)...)
		return
	}
	slog.Info("row-level security is ACTIVE — the application role is subject to RLS policies", append(attrs, "bypasses_rls", false)...)

	verifyTenantPin(comp, ctx)
}

// verifyTenantPin checks that the tenant pinned onto the runtime pool actually
// makes rows visible, and refuses to keep serving a platform that reads itself
// as empty.
//
// This is the gate that the 078/079 combination was missing: RLS was enabled and
// the runtime moved to a non-owner role, but nothing set app.tenant_id, so every
// tenant-scoped table returned zero rows and every write failed with SQLSTATE
// 42501 — while the process started "successfully" and the UI rendered an empty
// installation. Only "pinned" mode is checked: "off" is the documented rollback
// where inert RLS is the operator's explicit choice.
func verifyTenantPin(comp *bootstrap.Components, ctx context.Context) {
	pin := comp.DB.Pin()
	if pin.Mode != database.PinModePinned {
		return
	}

	attrs := []any{"tenant_id", pin.TenantID, "rls_tenant_mode", pin.Mode}

	pinnedGUC, err := comp.DB.CurrentTenantPin(ctx)
	if err != nil {
		failTenantPin(comp, attrs, "could not read the tenant GUC from a runtime connection", err)
		return
	}
	if pinnedGUC != pin.TenantID {
		failTenantPin(comp, attrs, fmt.Sprintf("runtime connections carry %q, not the pinned tenant", pinnedGUC), nil)
		return
	}

	// Roles are seeded unconditionally before this check, so an app role that sees
	// none of them is filtered out by RLS rather than facing an empty database.
	var visible int
	if err := comp.DB.QueryRow(ctx,
		`SELECT count(*) FROM roles WHERE tenant_id::text = $1`, pin.TenantID).Scan(&visible); err != nil {
		failTenantPin(comp, attrs, "cannot read the roles table through the runtime pool", err)
		return
	}
	if visible > 0 {
		slog.Info("RLS tenant pin verified", append(attrs, "visible_roles", visible)...)
		return
	}
	failTenantPin(comp, attrs, "the pinned tenant can see no roles although they were just seeded", nil)
}

// failTenantPin logs the diagnosis and exits, unless the operator declared the
// inert configuration acceptable via DB_RLS_ALLOW_INERT.
func failTenantPin(comp *bootstrap.Components, attrs []any, reason string, err error) {
	if err != nil {
		attrs = append(attrs, "error", err)
	}
	slog.Error("row-level security is active but the tenant pin does not make any row visible: "+reason,
		append(attrs,
			"role", comp.Config.Database.AppUser,
			"hint", "check DB_RLS_TENANT_MODE / DB_SINGLE_TENANT_ID; to run with inert RLS set DB_RLS_TENANT_MODE=off",
		)...)
	if allowInertEnv() {
		slog.Warn("continuing anyway because DB_RLS_ALLOW_INERT=true; the platform will read itself as empty")
		return
	}
	os.Exit(1)
}

func allowInertEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DB_RLS_ALLOW_INERT")))
	return v == "1" || v == "true" || v == "yes"
}
