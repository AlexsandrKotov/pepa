package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — root application configuration
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Redis         RedisConfig         `mapstructure:"redis"`
	S3            S3Config            `mapstructure:"s3"`
	Auth          AuthConfig          `mapstructure:"auth"`
	Plugin        PluginConfig        `mapstructure:"plugin"`
	AI            AIConfig            `mapstructure:"ai"`
	CORS          CORSConfig          `mapstructure:"cors"`
	Worker        WorkerConfig        `mapstructure:"worker"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	// HostDataDir is the root directory on the host containing all
	// scannable/deployable projects. When PEPA runs inside Docker this
	// directory is bind-mounted into the container at the same path so that
	// Trivy scans, Terraform local_path, Docker Compose folder_path, and
	// blueprint deployments can access host files. Empty means native mode
	// (no path translation).
	HostDataDir string `mapstructure:"host_data_dir"`
}

type ServerConfig struct {
	Port         string `mapstructure:"port"`
	Host         string `mapstructure:"host"`
	Env          string `mapstructure:"env"`
	LogLevel     string `mapstructure:"log_level"`
	PlatformName string `mapstructure:"platform_name"`
	BaseURL      string `mapstructure:"base_url"`
	PprofEnabled bool   `mapstructure:"pprof_enabled"`
	PprofPort    int    `mapstructure:"pprof_port"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	DBName   string `mapstructure:"dbname"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	SSLMode  string `mapstructure:"sslmode"`
	// AppUser is the non-superuser role used for runtime queries.
	// When set, the application connects as this role so that
	// row-level-security policies actually apply. Migrations still
	// run as User (the table owner / superuser).
	AppUser     string `mapstructure:"app_user"`
	AppPassword string `mapstructure:"app_password"`
	// RLSTenantMode decides how the app.tenant_id GUC that every RLS policy
	// reads is populated. "off" leaves it unset, which makes the policies
	// unable to match any row — only valid when the runtime role bypasses RLS.
	// "pinned" sets the GUC once per pooled connection to SingleTenantID, so a
	// single-tenant deployment gets working row-level security without
	// request-scoped connection pinning. "per_request" is reserved for
	// request-scoped pinning and is rejected until that exists.
	// Empty means: "pinned" when an app role is configured, otherwise "off".
	RLSTenantMode string `mapstructure:"rls_tenant_mode"`
	// SingleTenantID is the tenant pinned onto every runtime connection in
	// "pinned" mode. Empty resolves to database.DefaultTenantID.
	SingleTenantID string `mapstructure:"single_tenant_id"`
}

// RLS tenant-pinning modes. See DatabaseConfig.RLSTenantMode.
const (
	RLSTenantModeOff        = "off"
	RLSTenantModePinned     = "pinned"
	RLSTenantModePerRequest = "per_request"
)

func (d DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

// RuntimeConnectionString returns the connection string for the application
// role (AppUser). Falls back to the owner connection when AppUser is empty
// or matches User.
func (d DatabaseConfig) RuntimeConnectionString() string {
	if d.AppUser == "" || d.AppUser == d.User {
		return d.ConnectionString()
	}
	pw := d.AppPassword
	if pw == "" {
		pw = d.Password
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.AppUser, pw, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

// HasAppRole reports whether a separate application role is configured.
func (d DatabaseConfig) HasAppRole() bool {
	return d.AppUser != "" && d.AppUser != d.User
}

// EffectiveRLSTenantMode returns the RLS tenant-pinning mode, defaulting to
// "pinned" whenever runtime queries run under the app role (otherwise those
// queries would be subject to policies that can never match) and "off"
// otherwise.
func (d DatabaseConfig) EffectiveRLSTenantMode() string {
	mode := strings.ToLower(strings.TrimSpace(d.RLSTenantMode))
	if mode != "" {
		return mode
	}
	if d.HasAppRole() {
		return RLSTenantModePinned
	}
	return RLSTenantModeOff
}

// ValidateRLS rejects configurations where row-level security is nominally on
// but the tenant GUC cannot work: "per_request" is not implemented, and
// "pinned" without an app role pins nothing useful.
//
// "off" under an app role is deliberately not an error here — it is the documented
// emergency rollback (RLS goes back to being inert). The startup self-check in
// cmd/api-server logs that state loudly instead.
func (d DatabaseConfig) ValidateRLS() error {
	switch d.EffectiveRLSTenantMode() {
	case RLSTenantModeOff:
		return nil
	case RLSTenantModePinned:
		if !d.HasAppRole() {
			return fmt.Errorf(
				"DB_RLS_TENANT_MODE=%s only makes sense with a non-owner runtime role; set APP_POSTGRES_USER or use %s",
				RLSTenantModePinned, RLSTenantModeOff)
		}
		// An empty SingleTenantID resolves to database.DefaultTenantID, which is
		// exactly what a pinned single-tenant deployment means.
		return nil
	case RLSTenantModePerRequest:
		return fmt.Errorf(
			"DB_RLS_TENANT_MODE=%s is not implemented: repositories take connections straight from the pool, so a "+
				"request-scoped GUC cannot be pinned yet. Use %s for single-tenant deployments.",
			RLSTenantModePerRequest, RLSTenantModePinned)
	default:
		return fmt.Errorf("unknown DB_RLS_TENANT_MODE %q (want %s, %s or %s)", d.RLSTenantMode,
			RLSTenantModeOff, RLSTenantModePinned, RLSTenantModePerRequest)
	}
}

// RLSTenantPin is the resolved tenant-pinning configuration for the runtime
// connection pool: the effective mode plus the tenant to pin. An empty tenantID
// means "not configured" and is up to the caller to default to the platform's
// built-in tenant, so this package stays free of database imports.
func (d DatabaseConfig) RLSTenantPin() (mode string, tenantID string) {
	return d.EffectiveRLSTenantMode(), strings.TrimSpace(d.SingleTenantID)
}

type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

type S3Config struct {
	Endpoint      string `mapstructure:"endpoint"`
	AccessKey     string `mapstructure:"access_key"`
	SecretKey     string `mapstructure:"secret_key"`
	UseSSL        bool   `mapstructure:"use_ssl"`
	BucketPlugins string `mapstructure:"bucket_plugins"`
}

type AuthConfig struct {
	JWTSecret       string        `mapstructure:"jwt_secret"`
	SessionDuration time.Duration `mapstructure:"session_duration"`
	BCryptCost      int           `mapstructure:"bcrypt_cost"`
	TokenExpiry     time.Duration `mapstructure:"token_expiry"`
	RefreshExpiry   time.Duration `mapstructure:"refresh_expiry"`
	OIDC            OIDCConfig    `mapstructure:"oidc"`
	AzureAD         AzureADConfig `mapstructure:"azure_ad"`
	LDAP            LDAPConfig    `mapstructure:"ldap"`
	Google          GoogleConfig  `mapstructure:"google"`
	GitHub          GitHubConfig  `mapstructure:"github"`
}

type OIDCConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	Issuer       string   `mapstructure:"issuer"`
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
	RedirectURL  string   `mapstructure:"redirect_url"`
	Scopes       []string `mapstructure:"scopes"`
}

// AzureADConfig holds Azure AD (Microsoft Entra ID) OIDC settings.
// Azure AD uses the standard OIDC flow with Microsoft-specific endpoints.
type AzureADConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	TenantID     string `mapstructure:"tenant_id"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
}

// LDAPConfig holds LDAP/Active Directory authentication settings.
type LDAPConfig struct {
	Enabled            bool              `mapstructure:"enabled"`
	URL                string            `mapstructure:"url"`
	BindDN             string            `mapstructure:"bind_dn"`
	BindPassword       string            `mapstructure:"bind_password"`
	BaseDN             string            `mapstructure:"base_dn"`
	UserFilter         string            `mapstructure:"user_filter"`
	GroupFilter        string            `mapstructure:"group_filter"`
	EmailAttr          string            `mapstructure:"email_attr"`
	NameAttr           string            `mapstructure:"name_attr"`
	StartTLS           bool              `mapstructure:"start_tls"`
	InsecureSkipVerify bool              `mapstructure:"insecure_skip_verify"`
	CACertificate      string            `mapstructure:"ca_certificate"`
	GroupMapping       map[string]string `mapstructure:"group_mapping"`
}

// GoogleConfig holds Google OAuth2 authentication settings.
type GoogleConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
}

// GitHubConfig holds GitHub OAuth2 authentication settings.
type GitHubConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
}

type PluginConfig struct {
	Dir              string `mapstructure:"dir"`
	CustomDir        string `mapstructure:"custom_dir"`
	LogLevel         string `mapstructure:"log_level"`
	SignatureVerify  bool   `mapstructure:"signature_verify"`
	SignatureEnforce bool   `mapstructure:"signature_enforce"`
}

type AIConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	DefaultProvider string `mapstructure:"default_provider"`
	OpenAIAPIKey    string `mapstructure:"openai_api_key"`
	OllamaBaseURL   string `mapstructure:"ollama_base_url"`
}

type CORSConfig struct {
	Origins []string `mapstructure:"origins"`
}

type WorkerConfig struct {
	Concurrency int `mapstructure:"concurrency"`
}

// ObservabilityConfig holds OpenTelemetry and observability settings.
// Controls distributed tracing, metrics export, and log forwarding.
type ObservabilityConfig struct {
	Enabled      bool         `mapstructure:"enabled"`
	OTLPEndpoint string       `mapstructure:"otlp_endpoint"`
	ServiceName  string       `mapstructure:"service_name"`
	SamplingRate float64      `mapstructure:"sampling_rate"`
	Insecure     bool         `mapstructure:"insecure"`
	Syslog       SyslogConfig `mapstructure:"syslog"`
}

// SyslogConfig holds syslog forwarding settings.
type SyslogConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Network  string `mapstructure:"network"`  // "udp", "tcp"
	Address  string `mapstructure:"address"`  // "syslog-server:514"
	Tag      string `mapstructure:"tag"`      // "pepa"
	Facility string `mapstructure:"facility"` // "local0"-"local7"
}

// DefaultConfig returns a configuration with sensible defaults for development.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         "8080",
			Host:         "127.0.0.1",
			Env:          "development",
			LogLevel:     "info",
			PlatformName: "PEPA",
			BaseURL:      "http://localhost:8088",
			PprofEnabled: false,
			PprofPort:    6060,
		},
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     5432,
			DBName:   "pepa",
			User:     "pepa",
			Password: "pepa_dev",
			SSLMode:  "disable",
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			Password: "redis_dev",
			DB:       0,
		},
		S3: S3Config{
			Endpoint:      "", // empty = use local storage; set S3_ENDPOINT to enable S3
			AccessKey:     "",
			SecretKey:     "",
			UseSSL:        false,
			BucketPlugins: "pepa-plugins",
		},
		Auth: AuthConfig{ //nolint:gosec // #nosec // G101: struct contains dev default JWT secret; Validate() warns if not overridden
			JWTSecret:       "dev-jwt-secret-change-in-production", //nolint:gosec // #nosec // G101: dev default; Validate() warns if not overridden
			SessionDuration: 24 * time.Hour,                        // 24 hours
			TokenExpiry:     24 * time.Hour,                        // 24 hours
			RefreshExpiry:   7 * 24 * time.Hour,                    // 7 days
			BCryptCost:      10,
			OIDC: OIDCConfig{
				Enabled: false,
				Scopes:  []string{"openid", "profile", "email"},
			},
		},
		Plugin: PluginConfig{
			Dir:              "./plugins",
			LogLevel:         "info",
			SignatureVerify:  true, // verify plugin signatures on load
			SignatureEnforce: true, // reject unsigned plugins (set false for dev)
		},
		AI: AIConfig{
			Enabled:         false,
			DefaultProvider: "openai",
		},
		CORS: CORSConfig{
			Origins: []string{"http://localhost:3000"},
		},
		Worker: WorkerConfig{
			Concurrency: 3,
		},
		Observability: ObservabilityConfig{
			Enabled:      false,
			OTLPEndpoint: "",
			ServiceName:  "pepa-api",
			SamplingRate: 1.0,
			Insecure:     true,
			Syslog: SyslogConfig{
				Enabled:  false,
				Network:  "udp",
				Address:  "",
				Tag:      "pepa",
				Facility: "local0",
			},
		},
	}
}

// LoadFromEnv populates config from environment variables.
func (c *Config) LoadFromEnv() {
	if v := getenv("SERVER_PORT"); v != "" {
		c.Server.Port = v
	}
	if v := getenv("SERVER_HOST"); v != "" {
		c.Server.Host = v
	}
	if v := getenv("SERVER_ENV"); v != "" {
		c.Server.Env = v
	}
	if v := getenv("SERVER_LOG_LEVEL"); v != "" {
		c.Server.LogLevel = v
	}
	if v := getenv("PLATFORM_NAME"); v != "" {
		c.Server.PlatformName = v
	}
	if v := getenv("BASE_URL"); v != "" {
		c.Server.BaseURL = v
	}

	// Database
	if v := getenv("POSTGRES_HOST"); v != "" {
		c.Database.Host = v
	}
	if v := getenv("POSTGRES_PORT"); v != "" {
		if port, err := fmt.Sscanf(v, "%d", &c.Database.Port); err == nil && port == 1 {
			// Successfully parsed port
		}
	}
	if v := getenv("POSTGRES_DB"); v != "" {
		c.Database.DBName = v
	}
	if v := getenv("POSTGRES_USER"); v != "" {
		c.Database.User = v
	}
	if v := getenv("POSTGRES_PASSWORD"); v != "" {
		c.Database.Password = v
	}
	if v := getenv("POSTGRES_SSLMODE"); v != "" {
		c.Database.SSLMode = v
	}
	// Application role (non-superuser for active RLS enforcement).
	if v := getenv("APP_POSTGRES_USER"); v != "" {
		c.Database.AppUser = v
	}
	if v := getenv("APP_POSTGRES_PASSWORD"); v != "" {
		c.Database.AppPassword = v
	}
	// How the app.tenant_id GUC behind every RLS policy gets populated.
	if v := getenv("DB_RLS_TENANT_MODE"); v != "" {
		c.Database.RLSTenantMode = strings.ToLower(strings.TrimSpace(v))
	}
	if v := getenv("DB_SINGLE_TENANT_ID"); v != "" {
		c.Database.SingleTenantID = strings.TrimSpace(v)
	}

	// Redis
	if v := getenv("REDIS_HOST"); v != "" {
		c.Redis.Host = v
	}
	if v := getenv("REDIS_PORT"); v != "" {
		if port, err := fmt.Sscanf(v, "%d", &c.Redis.Port); err == nil && port == 1 {
			// Successfully parsed port
		}
	}
	if v := getenv("REDIS_PASSWORD"); v != "" {
		c.Redis.Password = v
	}

	// S3
	if v := getenv("S3_ENDPOINT"); v != "" {
		c.S3.Endpoint = v
	}
	if v := getenv("S3_ACCESS_KEY"); v != "" {
		c.S3.AccessKey = v
	}
	if v := getenv("S3_SECRET_KEY"); v != "" {
		c.S3.SecretKey = v
	}

	// Auth
	if v := getenv("AUTH_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	if v := getenv("AUTH_SESSION_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Auth.SessionDuration = d
		}
	}
	if v := getenv("AUTH_TOKEN_EXPIRY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Auth.TokenExpiry = d
		}
	}
	if v := getenv("AUTH_REFRESH_EXPIRY"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			c.Auth.RefreshExpiry = d
		}
	}
	if v := getenv("AUTH_BCRYPT_COST"); v != "" {
		var cost int
		if n, err := fmt.Sscanf(v, "%d", &cost); err == nil && n == 1 {
			if cost < 4 {
				cost = 4
			}
			if cost > 16 {
				slog.Warn("AUTH_BCRYPT_COST too high, clamping to 16", "cost", cost)
				cost = 16
			}
			c.Auth.BCryptCost = cost
		}
	}

	// Google OAuth
	if v := getenv("GOOGLE_OAUTH_ENABLED"); v == "true" {
		c.Auth.Google.Enabled = true
	}
	if v := getenv("GOOGLE_CLIENT_ID"); v != "" {
		c.Auth.Google.ClientID = v
	}
	if v := getenv("GOOGLE_CLIENT_SECRET"); v != "" {
		c.Auth.Google.ClientSecret = v
	}
	if v := getenv("GOOGLE_REDIRECT_URL"); v != "" {
		c.Auth.Google.RedirectURL = v
	}

	// GitHub OAuth
	if v := getenv("GITHUB_OAUTH_ENABLED"); v == "true" {
		c.Auth.GitHub.Enabled = true
	}
	if v := getenv("GITHUB_CLIENT_ID"); v != "" {
		c.Auth.GitHub.ClientID = v
	}
	if v := getenv("GITHUB_CLIENT_SECRET"); v != "" {
		c.Auth.GitHub.ClientSecret = v
	}
	if v := getenv("GITHUB_REDIRECT_URL"); v != "" {
		c.Auth.GitHub.RedirectURL = v
	}

	// Plugin
	if v := getenv("PLUGIN_DIR"); v != "" {
		c.Plugin.Dir = v
	}
	if v := getenv("CUSTOM_PLUGIN_DIR"); v != "" {
		c.Plugin.CustomDir = v
	}
	if v := getenv("PLUGIN_SIGNATURE_VERIFY"); v != "" {
		c.Plugin.SignatureVerify = (v == "true")
	}
	if v := getenv("PLUGIN_SIGNATURE_ENFORCE"); v != "" {
		c.Plugin.SignatureEnforce = (v == "true")
	}

	// AI
	if v := getenv("AI_ENABLED"); v == "true" {
		c.AI.Enabled = true
	}
	if v := getenv("AI_DEFAULT_PROVIDER"); v != "" {
		c.AI.DefaultProvider = v
	}
	if v := getenv("AI_OPENAI_API_KEY"); v != "" {
		c.AI.OpenAIAPIKey = v
	}
	if v := getenv("AI_OLLAMA_BASE_URL"); v != "" {
		c.AI.OllamaBaseURL = v
	}

	// CORS
	if v := getenv("CORS_ORIGINS"); v != "" {
		c.CORS.Origins = strings.Split(v, ",")
	}

	// Observability (OpenTelemetry)
	if v := getenv("OTEL_ENABLED"); v == "true" {
		c.Observability.Enabled = true
	}
	if v := getenv("OTEL_ENDPOINT"); v != "" {
		c.Observability.OTLPEndpoint = v
	}
	if v := getenv("OTEL_SERVICE_NAME"); v != "" {
		c.Observability.ServiceName = v
	}
	if v := getenv("OTEL_SAMPLING_RATE"); v != "" {
		if rate, err := strconv.ParseFloat(v, 64); err == nil && rate >= 0 && rate <= 1 {
			c.Observability.SamplingRate = rate
		}
	}
	if v := getenv("OTEL_INSECURE"); v == "true" {
		c.Observability.Insecure = true
	}

	// Syslog
	if v := getenv("SYSLOG_ENABLED"); v == "true" {
		c.Observability.Syslog.Enabled = true
	}
	if v := getenv("SYSLOG_NETWORK"); v != "" {
		c.Observability.Syslog.Network = v
	}
	if v := getenv("SYSLOG_ADDRESS"); v != "" {
		c.Observability.Syslog.Address = v
	}
	if v := getenv("SYSLOG_TAG"); v != "" {
		c.Observability.Syslog.Tag = v
	}
	if v := getenv("SYSLOG_FACILITY"); v != "" {
		c.Observability.Syslog.Facility = v
	}

	// Host data directory — root for all host filesystem access (scans, deploys).
	// HOST_DATA_DIR takes precedence; falls back to HOST_HOME_DIR for backward
	// compatibility with older deployments.
	if v := getenv("HOST_DATA_DIR"); v != "" {
		c.HostDataDir = v
	} else if v := getenv("HOST_HOME_DIR"); v != "" {
		c.HostDataDir = v
	}
}

func getenv(key string) string {
	return os.Getenv(key)
}

// knownInsecureDefaults maps config paths to their insecure default values.
// Used by Validate() to warn operators who forgot to override them.
//
//nolint:gosec // #nosec // G101: these are sentinel values for dev-mode warnings, not real credentials
var knownInsecureDefaults = map[string]string{
	"auth.jwt_secret":   "dev-jwt-secret-change-in-production",
	"database.password": "pepa_dev",
	"redis.password":    "redis_dev",
	"encryption_key":    "dev-secret-change-me-in-production",
}

// Validate checks whether the configuration is safe for production use.
// It returns a list of human-readable warnings (empty = all good).
func (c *Config) Validate() []string {
	var warnings []string

	for path, insecure := range knownInsecureDefaults {
		var actual string
		switch path {
		case "auth.jwt_secret":
			actual = c.Auth.JWTSecret
		case "database.password":
			actual = c.Database.Password
		case "redis.password":
			actual = c.Redis.Password
		case "encryption_key":
			actual = os.Getenv("ENCRYPTION_KEY")
			if actual == "" {
				actual = os.Getenv("AUTH_JWT_SECRET")
			}
			if actual == "" {
				actual = os.Getenv("JWT_SECRET")
			}
		}
		if actual == insecure {
			warnings = append(warnings, fmt.Sprintf(
				"INSECURE DEFAULT: %s is still set to the development value %q. Set the corresponding env var before deploying.",
				path, insecure,
			))
		}
	}

	if c.Server.Host == "0.0.0.0" {
		warnings = append(warnings, "server.host is 0.0.0.0 — the API will listen on all network interfaces. Use 127.0.0.1 and a reverse proxy in production.")
	}

	for _, origin := range c.CORS.Origins {
		if origin == "*" {
			warnings = append(warnings, "cors.origins contains \"*\" combined with Allow-Credentials: true. This allows any website to make authenticated requests.")
			break
		}
	}

	return warnings
}

// ValidateStrict returns an error if any critical secrets are still set to
// their insecure development defaults. This must be called before starting
// in production mode to prevent running with well-known credentials.
func (c *Config) ValidateStrict() error {
	var failures []string

	if c.Auth.JWTSecret == knownInsecureDefaults["auth.jwt_secret"] {
		failures = append(failures, "AUTH_JWT_SECRET is set to the development default — set a secure random value")
	}
	if c.Database.Password == knownInsecureDefaults["database.password"] {
		failures = append(failures, "POSTGRES_PASSWORD is set to the development default — set a secure random value")
	}
	if c.Redis.Password == knownInsecureDefaults["redis.password"] {
		failures = append(failures, "REDIS_PASSWORD is set to the development default — set a secure random value")
	}

	encKey := os.Getenv("ENCRYPTION_KEY")
	if encKey == "" {
		encKey = os.Getenv("AUTH_JWT_SECRET")
	}
	if encKey == "" || encKey == knownInsecureDefaults["encryption_key"] {
		failures = append(failures, "ENCRYPTION_KEY is not set or is the development default — set a secure random value")
	}

	if len(failures) > 0 {
		return fmt.Errorf("production mode requires secure credentials: %s", strings.Join(failures, "; "))
	}
	return nil
}
