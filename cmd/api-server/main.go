package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	"github.com/pepa/pepa/internal/service"
	"github.com/pepa/pepa/pkg/models"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	log.Printf("PEPA — Platform Engineering & Pipeline Automator starting... (version=%s, built=%s)", version, buildTime)

	// Bootstrap all shared components
	comp, err := bootstrap.Bootstrap()
	if err != nil {
		log.Fatalf("Bootstrap failed: %v", err)
	}

	// Auto-register plugins in DB
	comp.AutoRegisterPlugins()
	// Migrate plain text credentials to encrypted storage
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
		log.Printf("Warning: failed to seed default roles: %v", err)
	} else {
		log.Println("RBAC default roles seeded")
	}
	// Ensure all system roles have correct base permissions (idempotent).
	if err := rbacEngine.EnsureBasePermissions(context.Background(), defaultTenantID); err != nil {
		log.Printf("Warning: failed to ensure base permissions: %v", err)
	} else {
		log.Println("RBAC base permissions verified")
	}

	// Seed bootstrap token for first-run setup
	bootstrapDeps := rest.Dependencies{
		Config: comp.Config,
		DB:     comp.DB,
		Repos:  &rest.Repositories{},
		RBAC:   rbacEngine,
	}
	if rawToken, err := rest.SeedBootstrapToken(bootstrapDeps); err != nil {
		log.Printf("Warning: failed to seed bootstrap token: %v", err)
	} else if rawToken != "" {
		fmt.Fprintln(os.Stderr, "============================================================")
		fmt.Fprintln(os.Stderr, "  FIRST-RUN SETUP — Bootstrap Token Generated")
		fmt.Fprintln(os.Stderr, "============================================================")

		// Always write token to a file — never print to logs.
		// Logs are often captured by aggregation systems (ELK, Loki, CloudWatch)
		// where secrets can linger in indexes and be exposed to wider audiences.
		tokenPath := os.Getenv("BOOTSTRAP_TOKEN_PATH")
		if tokenPath == "" {
			tokenPath = "/var/run/pepa/bootstrap_token.txt" //nolint:gosec // G101: not a credential, just a default file path
		}
		if writeErr := os.MkdirAll(filepath.Dir(tokenPath), 0700); writeErr != nil { //nolint:gosec // G703: tokenPath is admin-controlled (env var or hardcoded default)
			log.Printf("Warning: failed to create token directory: %v", writeErr)
		}
		if writeErr := os.WriteFile(tokenPath, []byte(rawToken+"\n"), 0600); writeErr != nil { //nolint:gosec // G703: tokenPath is admin-controlled (env var or hardcoded default)
			log.Printf("Warning: failed to write bootstrap token file: %v", writeErr)
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
		},
		Services: &rest.Services{
			Deployment: service.NewDeploymentService(
				comp.ClusterRepo,
				comp.DeploymentRepo,
				comp.HelmRepo,
			),
			ServiceDeployment: service.NewServiceDeploymentService(
				comp.ClusterRepo,
				comp.ServiceRepo,
				comp.HelmRepo,
			),
			Connection: service.NewConnectionService(),
		},
		PluginMgr:        comp.PluginMgr,
		ProviderRegistry: comp.ProviderRegistry,
		PipelineRegistry: comp.PipelineRegistry,
		EventBus:         comp.EventBus,
		JobQueue:         comp.JobQueue,
		AIManager:        comp.AIManager,
		RBAC:             rbacEngine,
		Storage:          comp.Storage,
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

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("PEPA API listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	<-done
	log.Println("Shutting down gracefully...")

	// Write shutdown audit event
	writeSystemAuditEvent(comp, "shutdown", "system", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	comp.Shutdown(ctx)
	shutdownRouter()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("PEPA stopped")
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
		log.Printf("Warning: failed to write system audit event (%s): %v", action, err)
	}
}
