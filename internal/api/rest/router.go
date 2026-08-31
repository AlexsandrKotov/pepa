package rest

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/ai"
	"github.com/pepa/pepa/internal/auth"
	"github.com/pepa/pepa/internal/config"
	"github.com/pepa/pepa/internal/database"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/internal/gitops"
	"github.com/pepa/pepa/internal/observability"
	"github.com/pepa/pepa/internal/pipeline"
	"github.com/pepa/pepa/internal/plugin/engine"
	"github.com/pepa/pepa/internal/provider"
	"github.com/pepa/pepa/internal/queue"
	rbacengine "github.com/pepa/pepa/internal/rbac/engine"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/service"
	"github.com/pepa/pepa/internal/storage"
)

// Repositories groups all repository instances.
type Repositories struct {
	Entity          *repository.EntityRepository
	Workflow        *repository.WorkflowRepository
	Plugin          *repository.PluginRepository
	Scorecard       *repository.ScorecardRepository
	Audit           *repository.AuditRepository
	Cluster         *repository.ClusterRepository
	Deployment      *repository.DeploymentRepository
	Jira            *repository.JiraRepository
	Connection      *repository.ConnectionRepository
	Service         *repository.ServiceRepository
	Settings        *repository.SettingsRepository
	Environment     *repository.EnvironmentRepository
	EnvVariable     *repository.EnvironmentVariableRepository
	DockerHost      *repository.DockerHostRepository
	Helm            *repository.HelmRepository
	PipelineSource  *repository.PipelineSourceRepository
	PipelinePreset  *repository.PipelinePresetRepository
	PipelineRun     *repository.PipelineRunRepository
	Vault           *repository.VaultRepository
	VaultConfig     *repository.VaultConfigRepository
	Auth            *repository.AuthRepository
	TeamWorkflow    *repository.TeamWorkflowRepository
	GitopsRepo      *gitops.Repository
	UserCredential  *repository.UserCredentialRepository
	CredentialShare *repository.CredentialShareRepository
	Organization    *repository.OrganizationRepository
	RAG             *repository.RAGRepository
}

// Dependencies holds all injected dependencies for the HTTP layer.
type Dependencies struct {
	Config           *config.Config
	DB               *database.DB
	Repos            *Repositories
	Services         *Services
	PluginMgr        *engine.Manager
	ProviderRegistry *provider.Registry
	PipelineRegistry *pipeline.Registry
	EventBus         *events.Bus
	JobQueue         *queue.Queue
	AIManager        *ai.Manager
	IngestionEngine  *ai.IngestionEngine
	RAGPipeline      *ai.RAGPipeline
	RBAC             *rbacengine.Engine
	Storage          storage.Storage
	LoginLimiter     *auth.LoginRateLimiter
	Version          string
	BuildTime        string
}

// Services groups all service layer instances.
type Services struct {
	Deployment        *service.DeploymentService
	ServiceDeployment *service.ServiceDeploymentService
	Connection        *service.ConnectionService
}

// NewRouter creates and configures the Gin router with all routes.
// It returns the HTTP handler and a shutdown function that must be called
// to release background resources (rate limiter goroutines, etc.).
func NewRouter(deps Dependencies) (http.Handler, func()) {
	if deps.Config.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Validate JWT secret in production — refuse to start with insecure defaults.
	if deps.Config.Server.Env == "production" {
		if deps.Config.Auth.JWTSecret == "" || deps.Config.Auth.JWTSecret == "dev-jwt-secret-change-in-production" {
			slog.Error("AUTH_JWT_SECRET must be set to a secure random value in production (min 32 characters)")
			os.Exit(1)
		}
		if len(deps.Config.Auth.JWTSecret) < 32 {
			slog.Error("AUTH_JWT_SECRET must be at least 32 characters long")
			os.Exit(1)
		}
	}

	// Initialize login rate limiter if not already set.
	// 10 failed attempts per 15-minute window, 15-minute lockout.
	if deps.LoginLimiter == nil {
		deps.LoginLimiter = auth.NewLoginRateLimiter(10, 15*time.Minute, 15*time.Minute)
	}

	// Initialize Prometheus metrics
	metrics := observability.NewMetrics()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestIDMiddleware())
	r.Use(correlationMiddleware()) // Add trace ID to all requests
	r.Use(requestLogger())
	r.Use(metrics.GinMiddleware())
	r.Use(corsMiddleware(deps.Config.CORS))
	r.Use(securityHeadersMiddleware())
	r.Use(maxBodySizeMiddleware(100 << 20))          // 100 MB request body limit (plugin uploads)
	rateLimiter := newRateLimiter(1000, time.Minute) // 1000 req/min per IP
	r.Use(rateLimiter.Middleware())

	// Initialize bounded audit log workers.
	if deps.Repos.Audit != nil {
		initAuditWorkers(deps.Repos.Audit)
	}

	// Configure trusted proxies so that X-Forwarded-For / X-Real-IP headers
	// are only honoured when the request comes from a known reverse proxy.
	// In production, set TRUSTED_PROXY_CIDRS or rely on Gin's default (trust no proxy).
	if deps.Config.Server.Env == "production" {
		// Trust only loopback and private RFC1918 ranges (typical reverse-proxy setups).
		_ = r.SetTrustedProxies([]string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"})
	} else {
		// In development, trust all proxies for convenience.
		_ = r.SetTrustedProxies(nil)
	}

	// Health check (no auth)
	version := deps.Version
	if version == "" {
		version = "dev"
	}
	buildTime := deps.BuildTime
	if buildTime == "" {
		buildTime = "unknown"
	}

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"version": version,
			"app":     "PEPA — Platform Engineering & Pipeline Automator",
		})
	})

	// Readiness probe — checks dependencies (PostgreSQL, Redis)
	// K8s uses this to determine if the pod should receive traffic
	r.GET("/readyz", func(c *gin.Context) {
		ctx := c.Request.Context()
		checks := make(map[string]string)
		healthy := true

		// PostgreSQL check
		if deps.DB != nil && deps.DB.Pool != nil {
			if err := deps.DB.Pool.Ping(ctx); err != nil {
				checks["postgres"] = "unhealthy: " + err.Error()
				healthy = false
			} else {
				checks["postgres"] = "ok"
			}
		} else {
			checks["postgres"] = "not configured"
			healthy = false
		}

		// Redis check (via EventBus)
		if deps.EventBus != nil {
			if err := deps.EventBus.Ping(ctx); err != nil {
				checks["redis"] = "unhealthy: " + err.Error()
				healthy = false
			} else {
				checks["redis"] = "ok"
			}
		} else {
			checks["redis"] = "not configured"
		}

		status := http.StatusOK
		if !healthy {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"status": checks, "healthy": healthy})
	})

	// Prometheus metrics endpoint (no auth required)
	r.GET("/metrics", gin.WrapH(observability.Handler()))

	// Public auth routes (no JWT required)
	registerAuthRoutes(r, deps)

	// API v1 routes with JWT auth
	v1 := r.Group("/api/v1")
	v1.Use(bootstrapGuardMiddleware(deps))
	v1.Use(auth.Middleware(deps.Config.Auth.JWTSecret))
	v1.Use(rbacMiddleware(deps))
	v1.Use(apiAuditMiddleware(deps))
	{
		registerEntityRoutes(v1, deps)
		registerPluginRoutes(v1, deps)
		registerWorkflowRoutes(v1, deps)
		registerStepExecutionRoutes(v1, deps)
		registerScorecardRoutes(v1, deps)
		registerAuditRoutes(v1, deps)
		registerRBACRoutes(v1, deps)
		registerClusterRoutes(v1, deps)
		registerDeploymentRoutes(v1, deps)
		registerJiraRoutes(v1, deps)
		registerConnectionRoutes(v1, deps)
		registerServiceRoutes(v1, deps)
		registerCatalogRoutes(v1, deps)
		registerGitOpsRoutes(v1, deps)
		registerSettingsRoutes(v1, deps)
		registerEnvironmentRoutes(v1, deps)
		registerMarketplaceRoutes(v1, deps)
		registerDiscoveryRoutes(v1, deps)
		registerDockerHostRoutes(v1, deps)
		registerDockerServiceRoutes(v1, deps)
		registerHelmRepoRoutes(v1, deps)
		registerPipelineSourceRoutes(v1, deps)
		registerPipelineRunRoutes(v1, deps)
		registerPipelinePresetRoutes(v1, deps)
		registerVaultRoutes(v1, deps)
		registerStorageRoutes(v1, deps)
		registerS3BrowserRoutes(v1, deps)
		registerTeamRoutes(v1, deps)
		registerUserCredentialRoutes(v1, deps)
		registerServiceBlueprintRoutes(v1, deps)
		registerBlueprintGroupRoutes(v1, deps)
		registerBlueprintDeployRoutes(v1, deps)
		registerOrganizationRoutes(v1, deps)
		registerProxmoxRoutes(v1, deps)
		registerObservabilityRoutes(v1, deps)

		// System info
		v1.GET("/system/info", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"version":    version,
				"build_time": buildTime,
				"go_version": runtime.Version(),
				"plugins":    len(deps.PluginMgr.ListPlugins()),
			})
		})

		// AI endpoints
		if deps.AIManager != nil {
			aiHandlers := NewAIHandlers(deps.AIManager, deps)
			v1.POST("/ai/chat", aiHandlers.Chat)
			v1.POST("/ai/chat/stream", aiHandlers.ChatStream)
			v1.GET("/ai/status", aiHandlers.AIStatus)
			v1.GET("/ai/tools", aiHandlers.ListTools)
			v1.GET("/ai/history", aiHandlers.History)
			v1.GET("/ai/suggestions", aiHandlers.Suggestions)
			v1.POST("/ai/generate", aiHandlers.Generate)
			v1.POST("/ai/apply", aiHandlers.Apply)
			v1.POST("/ai/analyze", aiHandlers.Analyze)
			v1.POST("/ai/recommend", aiHandlers.Recommend)
			v1.PUT("/ai/default-provider", aiHandlers.SetDefaultProvider)

			// RAG knowledge base endpoints
			if deps.Repos != nil && deps.Repos.RAG != nil && deps.AIManager != nil {
				tenantID := uuid.MustParse(database.DefaultTenantID)
				ragHandlers := NewRAGHandlers(deps.Repos.RAG, deps.AIManager, tenantID)
				ragHandlers.SetIngestionEngine(deps.IngestionEngine)
				ragHandlers.SetPipeline(deps.RAGPipeline)
				v1.POST("/rag/ingest", ragHandlers.IngestDocument)
				v1.POST("/rag/search", ragHandlers.Search)
				v1.GET("/rag/documents", ragHandlers.ListDocuments)
				v1.GET("/rag/documents/:id", ragHandlers.GetDocument)
				v1.PUT("/rag/documents/:id", ragHandlers.UpdateDocument)
				v1.POST("/rag/documents", ragHandlers.CreateDocument)
				v1.DELETE("/rag/documents/:id", ragHandlers.DeleteDocument)
				v1.GET("/rag/stats", ragHandlers.GetStats)
				v1.POST("/rag/reindex", ragHandlers.Reindex)
				v1.POST("/rag/chat", ragHandlers.ChatWithRAG)
				v1.POST("/rag/chat/stream", ragHandlers.ChatStreamWithRAG)
			}

			// Proactive AI endpoints (risk assessment, doc generation, cost analysis)
			// These are registered but will return 503 if components aren't initialized.
			// Use the default tenant ID from constants instead of hardcoding.
			tenantID := uuid.MustParse(database.DefaultTenantID)
			proactiveHandlers := NewProactiveAIHandlers(nil, nil, nil, nil, tenantID)
			v1.POST("/ai/risk/assess", proactiveHandlers.AssessDeploymentRisk)
			v1.POST("/ai/docs/generate", proactiveHandlers.GenerateServiceDocs)
			v1.POST("/ai/docs/generate/:service", proactiveHandlers.GenerateServiceDocs)
			v1.GET("/ai/cost/analyze", proactiveHandlers.AnalyzeCosts)
			v1.GET("/ai/cost/stale", proactiveHandlers.DetectStaleResources)

			// NL Workflow Builder endpoint
			workflowBuilderHandlers := NewWorkflowBuilderHandlers(nil)
			v1.POST("/ai/workflow/build", workflowBuilderHandlers.BuildWorkflow)
			v1.POST("/ai/workflow/preview", workflowBuilderHandlers.PreviewWorkflow)

			// Multi-agent coordination endpoints
			multiAgentHandlers := NewMultiAgentHandlers(nil, nil)
			v1.POST("/ai/agents/route", multiAgentHandlers.Route)
			v1.POST("/ai/agents/coordinate", multiAgentHandlers.Coordinate)
			v1.GET("/ai/agents/specialists", multiAgentHandlers.ListSpecialists)

			// IDE webhook integration
			webhookHandlers := NewAIWebhookHandlers(deps.AIManager)
			v1.POST("/ai/webhook/suggest", webhookHandlers.Suggest)
			v1.GET("/ai/webhook/status", webhookHandlers.Status)
		}
	}

	// SSE real-time events (auth required)
	if deps.EventBus != nil {
		sseGroup := r.Group("/api/v1/events")
		sseGroup.Use(bootstrapGuardMiddleware(deps))
		sseGroup.Use(auth.Middleware(deps.Config.Auth.JWTSecret))
		registerSSERoutes(sseGroup, deps.EventBus)
	}

	return r, func() {
		rateLimiter.Stop()
	}
}

// bootstrapComplete is set to 1 once bootstrap is verified, so subsequent
// requests skip the SQL queries entirely.
var bootstrapComplete atomic.Bool

// bootstrapLastCheck tracks when we last queried DB for bootstrap status
// to avoid hammering the database on every request before bootstrap completes.
var bootstrapLastCheck atomic.Int64

// bootstrapGuardMiddleware blocks all API requests until the bootstrap token
// has been activated (first-run setup completed). Public auth endpoints are
// registered directly on the engine, not on the v1 group, so they are unaffected.
func bootstrapGuardMiddleware(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Fast path: once bootstrap is confirmed complete, skip all DB queries.
		if bootstrapComplete.Load() {
			c.Next()
			return
		}

		// Throttle: only check DB at most once per 5 seconds to prevent
		// a flood of requests from overwhelming the database before bootstrap.
		now := time.Now().Unix()
		last := bootstrapLastCheck.Load()
		if last > 0 && now-last < 5 {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":     "bootstrap required",
				"message":   "First-run setup has not been completed. Use the bootstrap token to activate the admin account.",
				"bootstrap": true,
			})
			c.Abort()
			return
		}
		bootstrapLastCheck.Store(now)

		// Check if there are any non-default users (real users besides the seeded admin).
		// Use a bounded timeout to prevent hung database connections from blocking
		// all requests indefinitely.
		bootstrapCtx, bootstrapCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer bootstrapCancel()

		var userCount int
		err := deps.DB.Pool.QueryRow(bootstrapCtx, `
			SELECT COUNT(*) FROM users WHERE id != '00000000-0000-0000-0000-000000000010'
		`).Scan(&userCount)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check bootstrap status"})
			c.Abort()
			return
		}
		if userCount == 0 {
			// Check if any bootstrap token has been used — definitive signal
			// that the bootstrap flow was already completed.
			var hasUsedToken bool
			_ = deps.DB.Pool.QueryRow(bootstrapCtx, `
				SELECT EXISTS(
					SELECT 1 FROM bootstrap_tokens WHERE used_at IS NOT NULL
				)
			`).Scan(&hasUsedToken)
			if !hasUsedToken {
				c.JSON(http.StatusForbidden, gin.H{
					"error":     "bootstrap required",
					"message":   "First-run setup has not been completed. Use the bootstrap token to activate the admin account.",
					"bootstrap": true,
				})
				c.Abort()
				return
			}
		}

		// Bootstrap is complete — cache the result so future requests skip the DB.
		bootstrapComplete.Store(true)
		c.Next()
	}
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader("X-Request-ID")
		if reqID == "" {
			reqID = uuid.New().String()[:8]
		}
		c.Set("request_id", reqID)
		c.Header("X-Request-ID", reqID)
		c.Next()
	}
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		reqID, _ := c.Get("request_id")
		slog.Info("http request",
			"request_id", reqID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start),
		)
	}
}

func corsMiddleware(cfg config.CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		for _, allowed := range cfg.Origins {
			// Never combine wildcard origin with credentials — reject "*" explicitly.
			if allowed == "*" {
				c.Header("Access-Control-Allow-Origin", "*")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
				// Omit Allow-Credentials when origin is wildcard.
				break
			}
			if allowed == origin {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
				c.Header("Access-Control-Allow-Credentials", "true")
				break
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// maxBodySizeMiddleware rejects requests whose body exceeds the given limit.
func maxBodySizeMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// securityHeadersMiddleware adds common security headers to all responses.
func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	}
}

// rateLimiter is a sharded per-IP token-bucket rate limiter.
// Sharding reduces mutex contention under high concurrency by splitting
// the IP table into independent segments, each with its own lock.
const rateLimiterShards = 16

type rateLimitShard struct {
	mu    sync.Mutex
	table map[string]*rateLimitEntry
}

type rateLimiter struct {
	shards [rateLimiterShards]*rateLimitShard
	limit  int
	window time.Duration
	stopCh chan struct{}
}

type rateLimitEntry struct {
	tokens    int
	lastReset time.Time
}

// newRateLimiter creates a rate limiter that allows `limit` requests per `window` per IP.
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		limit:  limit,
		window: window,
		stopCh: make(chan struct{}),
	}
	for i := range rl.shards {
		rl.shards[i] = &rateLimitShard{table: make(map[string]*rateLimitEntry)}
	}
	// Periodic cleanup of stale entries across all shards.
	go func() {
		ticker := time.NewTicker(5 * window)
		defer ticker.Stop()
		for {
			select {
			case <-rl.stopCh:
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-2 * window)
				for _, s := range rl.shards {
					s.mu.Lock()
					for ip, e := range s.table {
						if e.lastReset.Before(cutoff) {
							delete(s.table, ip)
						}
					}
					s.mu.Unlock()
				}
			}
		}
	}()
	return rl
}

// Stop shuts down the rate limiter cleanup goroutine.
func (rl *rateLimiter) Stop() {
	close(rl.stopCh)
}

// shard returns the shard for a given IP address.
func (rl *rateLimiter) shard(ip string) *rateLimitShard {
	// Fast hash: use last byte of IP string for distribution.
	// Good enough for rate-limiter sharding (not cryptographic).
	h := 0
	for i := 0; i < len(ip); i++ {
		h = h*31 + int(ip[i])
	}
	if h < 0 {
		h = -h
	}
	return rl.shards[h%rateLimiterShards]
}

// Middleware returns a Gin middleware that enforces the rate limit.
func (rl *rateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		s := rl.shard(ip)

		s.mu.Lock()
		entry, ok := s.table[ip]
		if !ok || now.Sub(entry.lastReset) > rl.window {
			s.table[ip] = &rateLimitEntry{tokens: rl.limit - 1, lastReset: now}
			s.mu.Unlock()
			c.Next()
			return
		}
		if entry.tokens <= 0 {
			s.mu.Unlock()
			c.Header("Retry-After", fmt.Sprintf("%d", int(rl.window.Seconds())))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded, try again later",
			})
			return
		}
		entry.tokens--
		s.mu.Unlock()
		c.Next()
	}
}
