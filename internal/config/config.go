package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Config — root application configuration
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Redis    RedisConfig    `mapstructure:"redis"`
	S3       S3Config       `mapstructure:"s3"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Plugin   PluginConfig   `mapstructure:"plugin"`
	AI       AIConfig       `mapstructure:"ai"`
	CORS     CORSConfig     `mapstructure:"cors"`
	Worker   WorkerConfig   `mapstructure:"worker"`
}

type ServerConfig struct {
	Port     string `mapstructure:"port"`
	Host     string `mapstructure:"host"`
	Env      string `mapstructure:"env"`
	LogLevel string `mapstructure:"log_level"`
}

type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	DBName   string `mapstructure:"dbname"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	SSLMode  string `mapstructure:"sslmode"`
}

func (d DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
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
}

type OIDCConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	Issuer       string   `mapstructure:"issuer"`
	ClientID     string   `mapstructure:"client_id"`
	ClientSecret string   `mapstructure:"client_secret"`
	RedirectURL  string   `mapstructure:"redirect_url"`
	Scopes       []string `mapstructure:"scopes"`
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

// DefaultConfig returns a configuration with sensible defaults for development.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:     "8080",
			Host:     "127.0.0.1",
			Env:      "development",
			LogLevel: "info",
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
		Auth: AuthConfig{
			JWTSecret:       "dev-jwt-secret-change-in-production", //nolint:gosec // G101: dev default; Validate() warns if not overridden
			SessionDuration: 24 * time.Hour,     // 24 hours
			TokenExpiry:     24 * time.Hour,     // 24 hours
			RefreshExpiry:   7 * 24 * time.Hour, // 7 days
			BCryptCost:      10,
			OIDC: OIDCConfig{
				Enabled: false,
				Scopes:  []string{"openid", "profile", "email"},
			},
		},
		Plugin: PluginConfig{
			Dir:             "./plugins",
			LogLevel:        "info",
			SignatureVerify: true, // verify plugin signatures on load (warn unless enforced)
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

	// Database
	if v := getenv("POSTGRES_HOST"); v != "" {
		c.Database.Host = v
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

	// Redis
	if v := getenv("REDIS_HOST"); v != "" {
		c.Redis.Host = v
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

	// Plugin
	if v := getenv("PLUGIN_DIR"); v != "" {
		c.Plugin.Dir = v
	}
	if v := getenv("CUSTOM_PLUGIN_DIR"); v != "" {
		c.Plugin.CustomDir = v
	}
	if v := getenv("PLUGIN_SIGNATURE_VERIFY"); v == "true" {
		c.Plugin.SignatureVerify = true
	}
	if v := getenv("PLUGIN_SIGNATURE_ENFORCE"); v == "true" {
		c.Plugin.SignatureEnforce = true
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
}

func getenv(key string) string {
	return os.Getenv(key)
}

// knownInsecureDefaults maps config paths to their insecure default values.
// Used by Validate() to warn operators who forgot to override them.
//
//nolint:gosec // G101: these are sentinel values for dev-mode warnings, not real credentials
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
