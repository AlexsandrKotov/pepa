package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/api/rest"
	"github.com/pepa/pepa/internal/bootstrap"
	"github.com/pepa/pepa/internal/database"
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

	// Run database migrations
	if err := comp.DB.RunMigrations(context.Background()); err != nil {
		slog.Error("database migrations failed", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations completed")

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

	// AI manager is initialized by bootstrap; available as comp.AIManager

	// Initialize HTTP router
	router, shutdownRouter := rest.NewRouter(rest.Dependencies{
		Config: comp.Config,
		DB:     comp.DB,
		Repos: &rest.Repositories{
			Entity:           comp.EntityRepo,
			Workflow:         comp.WorkflowRepo,
			Plugin:           comp.PluginRepo,
			Scorecard:        comp.ScorecardRepo,
			Audit:            comp.AuditRepo,
			Cluster:          comp.ClusterRepo,
			Deployment:       comp.DeploymentRepo,
			Jira:             comp.JiraRepo,
			Connection:       comp.ConnectionRepo,
			Service:          comp.ServiceRepo,
			Settings:         comp.SettingsRepo,
			Environment:      comp.EnvironmentRepo,
			EnvVariable:      comp.EnvVariableRepo,
			DockerHost:       comp.DockerHostRepo,
			Helm:             comp.HelmRepo,
			Registry:         comp.RegistryRepo,
			PipelineSource:   comp.PipelineSourceRepo,
			PipelinePreset:   comp.PipelinePresetRepo,
			PipelineRun:      comp.PipelineRunRepo,
			Vault:            comp.VaultRepo,
			VaultConfig:      comp.VaultConfigRepo,
			Auth:             comp.AuthRepo,
			TeamWorkflow:     comp.TeamWorkflowRepo,
			GitopsRepo:       comp.GitopsRepo,
			UserCredential:   comp.UserCredentialRepo,
			CredentialShare:  comp.CredentialShareRepo,
			Organization:     comp.OrganizationRepo,
			RAG:              comp.RAGRepo,
			SSHHost:          comp.SSHHostRepo,
			SSHHostGroup:     comp.SSHHostGroupRepo,
			PluginActivity:   comp.PluginActivityRepo,
			SecurityScan:     comp.SecurityScanRepo,
			ScanIgnore:       comp.ScanIgnoreRepo,
			DevOps:           comp.DevOpsRepo,
			NotificationRule: comp.NotificationRuleRepo,
			NotificationLog:  comp.NotificationLogRepo,
			GitOpsBinding:    comp.GitOpsBindingRepo,
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
			Connection: service.NewConnectionService(),
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
		},
		PluginMgr:        comp.PluginMgr,
		ProviderRegistry: comp.ProviderRegistry,
		PipelineRegistry: comp.PipelineRegistry,
		EventBus:         comp.EventBus,
		JobQueue:         comp.JobQueue,
		AIManager:        comp.AIManager,
		IngestionEngine:  comp.IngestionEngine,
		RAGPipeline:      comp.RAGPipeline,
		RBAC:             rbacEngine,
		Storage:          comp.Storage,
		Scanner:          security.NewScanner(comp.PluginMgr, comp.SecurityScanRepo, comp.ConnectionRepo, comp.RegistryRepo, comp.ScanIgnoreRepo),
		Version:          version,
		BuildTime:        buildTime,
	})

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
