// Package bootstrap provides shared initialization logic for all PEPA binaries
// (api-server, worker, CLI). It eliminates duplication of config loading,
// database/Redis connection, plugin discovery, and provider registry setup.
package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/ai"
	"github.com/pepa/pepa/internal/config"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/internal/gitops"
	"github.com/pepa/pepa/internal/logging"
	"github.com/pepa/pepa/internal/pipeline"
	"github.com/pepa/pepa/internal/plugin/engine"
	"github.com/pepa/pepa/internal/provider"
	"github.com/pepa/pepa/internal/queue"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/storage"
)

// Components holds all initialized core components shared across binaries.
type Components struct {
	Config           *config.Config
	DB               *database.DB
	Redis            *database.Redis
	Storage          storage.Storage
	PluginMgr        *engine.Manager
	ProviderRegistry *provider.Registry
	EventBus         *events.Bus
	JobQueue         *queue.Queue

	// Repositories
	EntityRepo          *repository.EntityRepository
	WorkflowRepo        *repository.WorkflowRepository
	PluginRepo          *repository.PluginRepository
	ScorecardRepo       *repository.ScorecardRepository
	AuditRepo           *repository.AuditRepository
	ClusterRepo         *repository.ClusterRepository
	DeploymentRepo      *repository.DeploymentRepository
	JiraRepo            *repository.JiraRepository
	ConnectionRepo      *repository.ConnectionRepository
	ServiceRepo         *repository.ServiceRepository
	SettingsRepo        *repository.SettingsRepository
	EnvironmentRepo     *repository.EnvironmentRepository
	EnvVariableRepo     *repository.EnvironmentVariableRepository
	DockerHostRepo      *repository.DockerHostRepository
	HelmRepo            *repository.HelmRepository
	PipelineSourceRepo  *repository.PipelineSourceRepository
	PipelinePresetRepo  *repository.PipelinePresetRepository
	PipelineRunRepo     *repository.PipelineRunRepository
	VaultRepo           *repository.VaultRepository
	VaultConfigRepo     *repository.VaultConfigRepository
	AuthRepo            *repository.AuthRepository
	TeamWorkflowRepo    *repository.TeamWorkflowRepository
	GitopsRepo          *gitops.Repository
	UserCredentialRepo  *repository.UserCredentialRepository
	CredentialShareRepo *repository.CredentialShareRepository
	OrganizationRepo    *repository.OrganizationRepository

	// Pipeline
	PipelineRegistry *pipeline.Registry

	// AI
	AIManager *ai.Manager
}

// Bootstrap loads configuration, connects to infrastructure, discovers plugins,
// and initializes all shared components. Callers must call Shutdown() when done.
func Bootstrap() (*Components, error) {
	// Load configuration
	cfg := config.DefaultConfig()
	cfg.LoadFromEnv()

	// Initialize structured logging
	logging.Init(cfg.Server.Env, cfg.Server.LogLevel)

	// Warn about insecure defaults
	for _, w := range cfg.Validate() {
		slog.Warn("insecure default detected", "warning", w)
	}

	// Validate encryption key strength
	isProduction := cfg.Server.Env == "production"
	if err := crypto.ValidateKeyStrength(isProduction); err != nil {
		if isProduction {
			return nil, fmt.Errorf("encryption key validation failed: %w", err)
		}
		slog.Warn("encryption key validation warning", "error", err)
	} else {
		slog.Debug("encryption key validation passed")
	}

	// Initialize PostgreSQL
	db, err := database.New(cfg.Database.ConnectionString())
	if err != nil {
		return nil, err
	}
	slog.Info("PostgreSQL connected", "host", cfg.Database.Host, "port", cfg.Database.Port, "db", cfg.Database.DBName)

	// Run database migrations
	if err := db.RunMigrations(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("database migrations up to date")

	// Initialize Redis
	redis, err := database.NewRedis(cfg.Redis.Addr(), cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		db.Close()
		return nil, err
	}
	slog.Info("Redis connected", "addr", cfg.Redis.Addr())

	// Initialize plugin storage
	// S3 is optional: set S3_ENDPOINT to enable, otherwise local filesystem is used.
	var pluginStorage storage.Storage
	if cfg.S3.Endpoint != "" {
		s3Client, err := storage.NewS3Client(cfg.S3)
		if err != nil {
			slog.Warn("S3 storage unavailable", "error", err)
		} else {
			if err := s3Client.EnsureBuckets(context.Background()); err != nil {
				slog.Warn("S3 bucket init failed", "error", err)
			} else {
				pluginStorage = s3Client
				slog.Info("using S3 storage for plugin binaries")
			}
		}
	}
	if pluginStorage == nil {
		// Default: local filesystem storage (uses Docker volume in containers)
		customDir := cfg.Plugin.CustomDir
		if customDir == "" {
			customDir = "./custom-plugins"
		}
		localStorage, err := storage.NewLocalStorage(customDir)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("init local storage: %w", err)
		}
		pluginStorage = localStorage
		slog.Info("using local filesystem storage for plugin binaries")
	}

	// Initialize plugin manager and discover plugins
	pluginMgr := engine.NewManager(cfg.Plugin, db)
	providerRegistry := provider.NewRegistry()

	// Initialize event bus and job queue
	eventBus := events.NewBus(redis.Client)
	jobQueue := queue.New(redis.Client)

	// Initialize all repositories
	c := &Components{
		Config:              cfg,
		DB:                  db,
		Redis:               redis,
		Storage:             pluginStorage,
		PluginMgr:           pluginMgr,
		ProviderRegistry:    providerRegistry,
		EventBus:            eventBus,
		JobQueue:            jobQueue,
		EntityRepo:          repository.NewEntityRepository(db),
		WorkflowRepo:        repository.NewWorkflowRepository(db),
		PluginRepo:          repository.NewPluginRepository(db),
		ScorecardRepo:       repository.NewScorecardRepository(db),
		AuditRepo:           repository.NewAuditRepository(db),
		ClusterRepo:         repository.NewClusterRepository(db),
		DeploymentRepo:      repository.NewDeploymentRepository(db),
		JiraRepo:            repository.NewJiraRepository(db),
		ConnectionRepo:      repository.NewConnectionRepository(db),
		ServiceRepo:         repository.NewServiceRepository(db),
		SettingsRepo:        repository.NewSettingsRepository(db),
		EnvironmentRepo:     repository.NewEnvironmentRepository(db),
		EnvVariableRepo:     repository.NewEnvironmentVariableRepository(db),
		DockerHostRepo:      repository.NewDockerHostRepository(db),
		HelmRepo:            repository.NewHelmRepository(db),
		PipelineSourceRepo:  repository.NewPipelineSourceRepository(db),
		PipelinePresetRepo:  repository.NewPipelinePresetRepository(db),
		PipelineRunRepo:     repository.NewPipelineRunRepository(db),
		VaultRepo:           repository.NewVaultRepository(db),
		VaultConfigRepo:     repository.NewVaultConfigRepository(db.Pool),
		AuthRepo:            repository.NewAuthRepository(db.Pool),
		TeamWorkflowRepo:    repository.NewTeamWorkflowRepository(db),
		GitopsRepo:          gitops.NewRepository(db),
		UserCredentialRepo:  repository.NewUserCredentialRepository(db.Pool),
		CredentialShareRepo: repository.NewCredentialShareRepository(db.Pool),
		OrganizationRepo:    repository.NewOrganizationRepository(db.Pool),
	}

	// Initialize pipeline provider registry
	pipelineRegistry := pipeline.NewRegistry()
	pipelineRegistry.Register("gitlab_ci", pipeline.NewGitLabAdapter())
	pipelineRegistry.Register("gitlab", pipeline.NewGitLabAdapter())
	pipelineRegistry.Register("ansible", pipeline.NewAnsibleAdapter())
	pipelineRegistry.Register("terraform", pipeline.NewTerraformAdapter())
	pipelineRegistry.Register("github_actions", pipeline.NewGitHubActionsAdapter())
	pipelineRegistry.Register("trivy", pipeline.NewTrivyAdapter())
	c.PipelineRegistry = pipelineRegistry
	slog.Info("pipeline registry initialized", "adapters", pipelineRegistry.List())

	// Load plugins asynchronously to avoid blocking API startup.
	// Each plugin spawns a subprocess with gRPC — loading 15+ plugins
	// can take 2+ minutes on slow disks. The provider registry is
	// thread-safe, so plugins become available as they finish loading.
	// AutoRegisterPlugins runs here (after discovery) to avoid a race
	// condition where it could execute before async discovery completes.
	go func() {
		if err := pluginMgr.DiscoverAndLoad(); err != nil {
			slog.Warn("plugin discovery failed", "error", err)
		}

		// Sync loaded plugins into the provider registry
		for name, info := range pluginMgr.ListLoadedPlugins() {
			grpcClient, err := pluginMgr.GetGRPCClient(name)
			if err != nil {
				slog.Warn("could not get gRPC client for plugin", "plugin", name, "error", err)
				continue
			}
			providerRegistry.Register(&provider.PluginEntry{
				Name:     name,
				Type:     info.PluginType,
				Info:     info,
				Executor: grpcClient,
				Enabled:  true,
			})
		}
		slog.Info("provider registry loaded", "count", len(providerRegistry.List()))

		// Reconcile discovered plugins with the database now that all
		// binaries are loaded and the provider registry is populated.
		c.AutoRegisterPlugins()
	}()

	// Initialize AI manager
	aiManager := ai.NewManager()
	if aiSettings, err := c.SettingsRepo.Get(context.Background(), "ai"); err == nil {
		var aiCfg struct {
			Enabled         bool   `json:"enabled"`
			DefaultProvider string `json:"default_provider"`
			Providers       map[string]struct {
				APIKey  string `json:"api_key"`
				BaseURL string `json:"base_url"`
				Model   string `json:"model"`
			} `json:"providers"`
		}
		if json.Unmarshal(aiSettings, &aiCfg) == nil && aiCfg.Enabled {
			for name, pCfg := range aiCfg.Providers {
				if pCfg.APIKey != "" || pCfg.BaseURL != "" {
					apiKey := pCfg.APIKey
					// Resolve vault references at startup
					if strings.HasPrefix(apiKey, "vault:") && c.VaultRepo != nil {
						raw := strings.TrimPrefix(apiKey, "vault:")
						if idx := strings.LastIndex(raw, "/"); idx >= 0 {
							secretPath := raw[:idx]
							secretKey := raw[idx+1:]
							secret, err := c.VaultRepo.GetAnyTenant(context.Background(), secretPath)
							if err != nil {
								slog.Warn("cannot resolve vault reference for AI provider", "provider", name, "error", err)
							} else if val, ok := secret.Data[secretKey]; ok {
								apiKey = val
							} else {
								slog.Warn("vault key not found in secret for AI provider", "key", secretKey, "path", secretPath, "provider", name)
							}
						}
					}
					if err := aiManager.ConfigureProvider(name, apiKey, pCfg.BaseURL, pCfg.Model); err != nil {
						slog.Warn("failed to configure AI provider", "provider", name, "error", err)
					}
				}
			}
			if aiCfg.DefaultProvider != "" {
				aiManager.SetDefaultProvider(aiCfg.DefaultProvider)
			}
			slog.Info("AI configured from settings", "providers", len(aiCfg.Providers))
		}
	}

	// Configure AI providers from AI connections. The Connections page is the
	// single place to configure AI providers, and connections take precedence
	// over legacy settings. The most recently created connection wins as default.
	if aiConns, err := c.ConnectionRepo.FindByTypeDecrypted(context.Background(), string(repository.ConnectionAI)); err == nil {
		sort.Slice(aiConns, func(i, j int) bool { return aiConns[i].CreatedAt.Before(aiConns[j].CreatedAt) })
		applied := 0
		for i := range aiConns {
			conn := aiConns[i]
			name, _ := conn.Config["provider"].(string)
			if name == "" {
				continue
			}
			apiKey, _ := conn.Config["api_key"].(string)
			baseURL, _ := conn.Config["base_url"].(string)
			model, _ := conn.Config["model"].(string)
			// Resolve vault references at startup
			if strings.HasPrefix(apiKey, "vault:") && c.VaultRepo != nil {
				raw := strings.TrimPrefix(apiKey, "vault:")
				if idx := strings.LastIndex(raw, "/"); idx >= 0 {
					secretPath := raw[:idx]
					secretKey := raw[idx+1:]
					if secret, err := c.VaultRepo.GetAnyTenant(context.Background(), secretPath); err != nil {
						slog.Warn("cannot resolve vault reference for AI connection", "connection", conn.Name, "error", err)
					} else if val, ok := secret.Data[secretKey]; ok {
						apiKey = val
					}
				}
			}
			if err := aiManager.ConfigureProvider(name, apiKey, baseURL, model); err != nil {
				slog.Warn("failed to configure AI provider from connection", "provider", name, "connection", conn.Name, "error", err)
				continue
			}
			aiManager.SetDefaultProvider(name)
			applied++
		}
		if applied > 0 {
			slog.Info("AI configured from connections", "providers", applied)
		}
	}
	c.AIManager = aiManager

	// Register agent tools (gives AI access to PEPA data)
	ai.RegisterAgentTools(aiManager.ToolRegistry(), &ai.AgentDeps{
		ServiceRepo:     c.ServiceRepo,
		DeploymentRepo:  c.DeploymentRepo,
		ClusterRepo:     c.ClusterRepo,
		PipelineSource:  c.PipelineSourceRepo,
		PipelineRun:     c.PipelineRunRepo,
		WorkflowRepo:    c.WorkflowRepo,
		EnvironmentRepo: c.EnvironmentRepo,
		ConnectionRepo:  c.ConnectionRepo,
		PluginRepo:      c.PluginRepo,
		EntityRepo:      c.EntityRepo,
		JiraRepo:        c.JiraRepo,
		DockerHostRepo:  c.DockerHostRepo,
		DBPool:          c.DB.Pool,
		TenantID:        uuid.MustParse(database.DefaultTenantID),
	})
	slog.Info("AI manager initialized", "tools", len(aiManager.ToolRegistry().List()))

	return c, nil
}

// AutoRegisterPlugins reconciles plugins discovered on disk with the database.
// Plugins with binaries on disk but no DB row are auto-registered (enabled,
// status=running) so the Marketplace correctly reflects their active state.
// Only explicitly "uninstalled" plugins are unloaded. Plugins that already
// have a DB row preserve the admin's enabled/disabled choice across restarts.
func (c *Components) AutoRegisterPlugins() {
	for name, info := range c.PluginMgr.ListLoadedPlugins() {
		existing, _ := c.PluginRepo.GetByName(context.Background(), name)

		if existing != nil && existing.Status == "uninstalled" {
			// Explicitly uninstalled by admin — keep unloaded.
			_ = c.PluginMgr.UnloadPlugin(name)
			c.ProviderRegistry.Unregister(name)
			slog.Info("plugin uninstalled, kept inactive", "plugin", name)
			continue
		}

		if existing == nil {
			// Plugin binary found on disk but no DB row — auto-register it
			// so the Marketplace shows it as installed (not orphaned).
			plugin := &repository.Plugin{
				Name:    name,
				Version: info.Version,
				Type:    info.PluginType,
				Status:  "running",
				Enabled: true,
			}
			if err := c.PluginRepo.Register(context.Background(), plugin); err != nil {
				slog.Warn("failed to auto-register plugin", "plugin", name, "error", err)
			} else {
				slog.Info("plugin auto-registered", "plugin", name, "version", info.Version)
			}
			continue
		}

		// Plugin already registered — preserve admin's enabled/disabled choice.
		// Sync the ProviderRegistry with the persisted state.
		if entry, ok := c.ProviderRegistry.Get(name); ok {
			entry.Enabled = existing.Enabled
		}

		if !existing.Enabled {
			// Installed but disabled by admin — keep the binary loaded, mark disabled.
			_ = c.PluginMgr.Disable(name)
			continue
		}

		_ = c.PluginMgr.Enable(name)
		// Reflect that the binary is loaded and running.
		if existing.Status != "running" || existing.Version != info.Version {
			existing.Status = "running"
			existing.Version = info.Version
			_ = c.PluginRepo.Register(context.Background(), existing)
		}
	}
}

// SeedDefaultHelmRepos adds well-known public Helm repositories if no repos exist yet.
func (c *Components) SeedDefaultHelmRepos() {
	if c.HelmRepo == nil {
		return
	}
	tenantID := uuid.MustParse(database.DefaultTenantID)
	existing, err := c.HelmRepo.List(context.Background(), tenantID)
	if err != nil {
		slog.Warn("failed to list helm repos for seeding", "error", err)
		return
	}
	if len(existing) > 0 {
		return // user already has repos configured
	}

	defaults := []repository.HelmRepo{
		{
			TenantID:    tenantID,
			Name:        "bitnami",
			Description: "Bitnami OSS Helm charts — popular open-source applications",
			RepoType:    "http",
			URL:         "https://charts.bitnami.com/bitnami",
			IsDefault:   true,
			Status:      "active",
		},
		{
			TenantID:    tenantID,
			Name:        "nginx",
			Description: "NGINX Ingress Controller and related charts",
			RepoType:    "http",
			URL:         "https://helm.nginx.com/stable",
			IsDefault:   false,
			Status:      "active",
		},
		{
			TenantID:    tenantID,
			Name:        "prometheus-community",
			Description: "Prometheus, Grafana, and monitoring stack charts",
			RepoType:    "http",
			URL:         "https://prometheus-community.github.io/helm-charts",
			IsDefault:   false,
			Status:      "active",
		},
		{
			TenantID:    tenantID,
			Name:        "jetstack",
			Description: "cert-manager and related TLS charts",
			RepoType:    "http",
			URL:         "https://charts.jetstack.io",
			IsDefault:   false,
			Status:      "active",
		},
	}

	seeded := 0
	for i := range defaults {
		if err := c.HelmRepo.Create(context.Background(), &defaults[i]); err != nil {
			slog.Warn("failed to seed default helm repo", "repo", defaults[i].Name, "error", err)
		} else {
			seeded++
		}
	}
	if seeded > 0 {
		slog.Info("seeded default public Helm repositories", "count", seeded)
	}
}

// StartEventBus begins listening for events.
func (c *Components) StartEventBus() {
	c.EventBus.Start()
	slog.Info("event bus started")
}

// Shutdown gracefully stops all components.
func (c *Components) Shutdown(ctx context.Context) {
	c.EventBus.Stop()
	c.PluginMgr.Shutdown(ctx)
	_ = c.Redis.Close()
	c.DB.Close()
}
