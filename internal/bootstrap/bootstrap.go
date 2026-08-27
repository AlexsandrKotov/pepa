// Package bootstrap provides shared initialization logic for all PEPA binaries
// (api-server, worker, CLI). It eliminates duplication of config loading,
// database/Redis connection, plugin discovery, and provider registry setup.
package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/ai"
	"github.com/pepa/pepa/internal/config"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/internal/gitops"
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
	EntityRepo           *repository.EntityRepository
	WorkflowRepo         *repository.WorkflowRepository
	PluginRepo           *repository.PluginRepository
	ScorecardRepo        *repository.ScorecardRepository
	AuditRepo            *repository.AuditRepository
	ClusterRepo          *repository.ClusterRepository
	DeploymentRepo       *repository.DeploymentRepository
	JiraRepo             *repository.JiraRepository
	ConnectionRepo       *repository.ConnectionRepository
	ServiceRepo          *repository.ServiceRepository
	SettingsRepo         *repository.SettingsRepository
	EnvironmentRepo      *repository.EnvironmentRepository
	EnvVariableRepo      *repository.EnvironmentVariableRepository
	DockerHostRepo       *repository.DockerHostRepository
	HelmRepo             *repository.HelmRepository
	PipelineSourceRepo   *repository.PipelineSourceRepository
	PipelinePresetRepo   *repository.PipelinePresetRepository
	PipelineRunRepo      *repository.PipelineRunRepository
	VaultRepo            *repository.VaultRepository
	VaultConfigRepo      *repository.VaultConfigRepository
	AuthRepo             *repository.AuthRepository
	TeamWorkflowRepo     *repository.TeamWorkflowRepository
	GitopsRepo           *gitops.Repository
	UserCredentialRepo   *repository.UserCredentialRepository
	CredentialShareRepo  *repository.CredentialShareRepository
	OrganizationRepo     *repository.OrganizationRepository

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

	// Warn about insecure defaults
	for _, w := range cfg.Validate() {
		log.Printf("WARNING: %s", w)
	}

	// Validate encryption key strength
	isProduction := cfg.Server.Env == "production"
	if err := crypto.ValidateKeyStrength(isProduction); err != nil {
		if isProduction {
			return nil, fmt.Errorf("encryption key validation failed: %w", err)
		}
		log.Printf("WARNING: Encryption key validation: %v", err)
	} else {
		log.Println("Encryption key validation passed")
	}

	// Initialize PostgreSQL
	db, err := database.New(cfg.Database.ConnectionString())
	if err != nil {
		return nil, err
	}
	log.Printf("PostgreSQL connected: %s:%d/%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)

	// Run database migrations
	if err := db.RunMigrations(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	log.Println("Database migrations up to date")

	// Initialize Redis
	redis, err := database.NewRedis(cfg.Redis.Addr(), cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		db.Close()
		return nil, err
	}
	log.Printf("Redis connected: %s", cfg.Redis.Addr())

	// Initialize plugin storage
	// S3 is optional: set S3_ENDPOINT to enable, otherwise local filesystem is used.
	var pluginStorage storage.Storage
	if cfg.S3.Endpoint != "" {
		s3Client, err := storage.NewS3Client(cfg.S3)
		if err != nil {
			log.Printf("Warning: S3 storage unavailable: %v", err)
		} else {
			if err := s3Client.EnsureBuckets(context.Background()); err != nil {
				log.Printf("Warning: S3 bucket init failed: %v", err)
			} else {
				pluginStorage = s3Client
				log.Println("Using S3 storage for plugin binaries")
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
		log.Println("Using local filesystem storage for plugin binaries")
	}

	// Initialize plugin manager and discover plugins
	pluginMgr := engine.NewManager(cfg.Plugin, db)
	providerRegistry := provider.NewRegistry()

	if err := pluginMgr.DiscoverAndLoad(); err != nil {
		log.Printf("Warning: plugin discovery failed: %v", err)
	}

	// Sync loaded plugins into the provider registry
	for name, info := range pluginMgr.ListLoadedPlugins() {
		grpcClient, err := pluginMgr.GetGRPCClient(name)
		if err != nil {
			log.Printf("Warning: could not get gRPC client for plugin %s: %v", name, err)
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
	log.Printf("Provider registry initialized with %d plugin(s)", len(providerRegistry.List()))

	// Initialize event bus and job queue
	eventBus := events.NewBus(redis.Client)
	jobQueue := queue.New(redis.Client)

	// Initialize all repositories
	c := &Components{
		Config:             cfg,
		DB:                 db,
		Redis:              redis,
		Storage:            pluginStorage,
		PluginMgr:          pluginMgr,
		ProviderRegistry:   providerRegistry,
		EventBus:           eventBus,
		JobQueue:           jobQueue,
		EntityRepo:         repository.NewEntityRepository(db),
		WorkflowRepo:       repository.NewWorkflowRepository(db),
		PluginRepo:         repository.NewPluginRepository(db),
		ScorecardRepo:      repository.NewScorecardRepository(db),
		AuditRepo:          repository.NewAuditRepository(db),
		ClusterRepo:        repository.NewClusterRepository(db),
		DeploymentRepo:     repository.NewDeploymentRepository(db),
		JiraRepo:           repository.NewJiraRepository(db),
		ConnectionRepo:     repository.NewConnectionRepository(db),
		ServiceRepo:        repository.NewServiceRepository(db),
		SettingsRepo:       repository.NewSettingsRepository(db),
		EnvironmentRepo:    repository.NewEnvironmentRepository(db),
		EnvVariableRepo:    repository.NewEnvironmentVariableRepository(db),
		DockerHostRepo:     repository.NewDockerHostRepository(db),
		HelmRepo:           repository.NewHelmRepository(db),
		PipelineSourceRepo: repository.NewPipelineSourceRepository(db),
		PipelinePresetRepo: repository.NewPipelinePresetRepository(db),
		PipelineRunRepo:    repository.NewPipelineRunRepository(db),
		VaultRepo:            repository.NewVaultRepository(db),
		VaultConfigRepo:      repository.NewVaultConfigRepository(db.Pool),
		AuthRepo:             repository.NewAuthRepository(db.Pool),
		TeamWorkflowRepo:     repository.NewTeamWorkflowRepository(db),
		GitopsRepo:           gitops.NewRepository(db),
		UserCredentialRepo:   repository.NewUserCredentialRepository(db.Pool),
		CredentialShareRepo:  repository.NewCredentialShareRepository(db.Pool),
		OrganizationRepo:     repository.NewOrganizationRepository(db.Pool),
	}

	// Initialize pipeline provider registry
	pipelineRegistry := pipeline.NewRegistry()
	pipelineRegistry.Register("gitlab_ci", pipeline.NewGitLabAdapter())
	pipelineRegistry.Register("gitlab", pipeline.NewGitLabAdapter())
	pipelineRegistry.Register("ansible", pipeline.NewAnsibleAdapter())
	pipelineRegistry.Register("terraform", pipeline.NewTerraformAdapter())
	c.PipelineRegistry = pipelineRegistry
	log.Printf("Pipeline registry initialized with adapters: %v", pipelineRegistry.List())

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
								log.Printf("Warning: cannot resolve vault reference for AI provider %s: %v", name, err)
							} else if val, ok := secret.Data[secretKey]; ok {
								apiKey = val
							} else {
								log.Printf("Warning: key %q not found in vault secret %q for AI provider %s", secretKey, secretPath, name)
							}
						}
					}
					if err := aiManager.ConfigureProvider(name, apiKey, pCfg.BaseURL, pCfg.Model); err != nil {
						log.Printf("Warning: failed to configure AI provider %s: %v", name, err)
					}
				}
			}
			if aiCfg.DefaultProvider != "" {
				aiManager.SetDefaultProvider(aiCfg.DefaultProvider)
			}
			log.Printf("AI configured from settings: %d provider(s)", len(aiCfg.Providers))
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
						log.Printf("Warning: cannot resolve vault reference for AI connection %s: %v", conn.Name, err)
					} else if val, ok := secret.Data[secretKey]; ok {
						apiKey = val
					}
				}
			}
			if err := aiManager.ConfigureProvider(name, apiKey, baseURL, model); err != nil {
				log.Printf("Warning: failed to configure AI provider %s from connection %s: %v", name, conn.Name, err)
				continue
			}
			aiManager.SetDefaultProvider(name)
			applied++
		}
		if applied > 0 {
			log.Printf("AI configured from connections: %d provider(s)", applied)
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
	log.Printf("AI manager initialized with %d tools", len(aiManager.ToolRegistry().List()))

	return c, nil
}

// AutoRegisterPlugins reconciles plugins discovered on disk with the database.
// Discovery alone does NOT activate a plugin: binaries that were never installed
// (or were uninstalled) are unloaded so they cannot serve any actions — an
// explicit install from the Marketplace is required. Plugins that are already
// registered keep the admin's enabled/disabled choice across restarts.
func (c *Components) AutoRegisterPlugins() {
	for name, info := range c.PluginMgr.ListLoadedPlugins() {
		existing, _ := c.PluginRepo.GetByName(context.Background(), name)

		if existing == nil || existing.Status == "uninstalled" {
			// Discovered on disk but never installed (or explicitly uninstalled).
			// Unload the subprocess and drop it from the provider registry so it
			// stays inactive until an admin installs it from the Marketplace.
			_ = c.PluginMgr.UnloadPlugin(name)
			c.ProviderRegistry.Unregister(name)
			log.Printf("[plugin-manager] plugin %s@%s discovered but not installed — kept inactive (install via Marketplace)", name, info.Version)
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
		log.Printf("Warning: failed to list helm repos for seeding: %v", err)
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
			log.Printf("Warning: failed to seed default helm repo %s: %v", defaults[i].Name, err)
		} else {
			seeded++
		}
	}
	if seeded > 0 {
		log.Printf("Seeded %d default public Helm repositories", seeded)
	}
}

// StartEventBus begins listening for events.
func (c *Components) StartEventBus() {
	c.EventBus.Start()
	log.Println("Event bus started")
}

// Shutdown gracefully stops all components.
func (c *Components) Shutdown(ctx context.Context) {
	c.EventBus.Stop()
	c.PluginMgr.Shutdown(ctx)
	c.Redis.Close()
	c.DB.Close()
}
