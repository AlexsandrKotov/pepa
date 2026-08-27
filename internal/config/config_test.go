package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.Server.Port != "8080" {
		t.Errorf("expected port 8080, got %s", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Env != "development" {
		t.Errorf("expected env development, got %s", cfg.Server.Env)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("expected db port 5432, got %d", cfg.Database.Port)
	}
	if cfg.Database.DBName != "pepa" {
		t.Errorf("expected dbname pepa, got %s", cfg.Database.DBName)
	}
	if cfg.Redis.Port != 6379 {
		t.Errorf("expected redis port 6379, got %d", cfg.Redis.Port)
	}
	if cfg.Worker.Concurrency != 3 {
		t.Errorf("expected worker concurrency 3, got %d", cfg.Worker.Concurrency)
	}
	if cfg.AI.Enabled {
		t.Error("expected AI disabled by default")
	}
	if cfg.Auth.SessionDuration != 24*time.Hour {
		t.Errorf("expected session duration 24h, got %v", cfg.Auth.SessionDuration)
	}
}

func TestDatabaseConfig_ConnectionString(t *testing.T) {
	d := DatabaseConfig{
		Host:     "db.example.com",
		Port:     5432,
		DBName:   "mydb",
		User:     "admin",
		Password: "secret",
		SSLMode:  "require",
	}
	got := d.ConnectionString()
	want := "postgres://admin:secret@db.example.com:5432/mydb?sslmode=require"
	if got != want {
		t.Errorf("ConnectionString() = %q, want %q", got, want)
	}
}

func TestDatabaseConfig_ConnectionString_Disable(t *testing.T) {
	d := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		DBName:   "test",
		User:     "user",
		Password: "pass",
		SSLMode:  "disable",
	}
	got := d.ConnectionString()
	if got != "postgres://user:pass@localhost:5432/test?sslmode=disable" {
		t.Errorf("unexpected connection string: %s", got)
	}
}

func TestRedisConfig_Addr(t *testing.T) {
	r := RedisConfig{Host: "redis.local", Port: 6380}
	got := r.Addr()
	want := "redis.local:6380"
	if got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set env vars
	envVars := map[string]string{
		"SERVER_PORT":         "9090",
		"SERVER_HOST":         "0.0.0.0",
		"SERVER_ENV":          "production",
		"SERVER_LOG_LEVEL":    "debug",
		"POSTGRES_HOST":       "pg-host",
		"POSTGRES_DB":         "prod_db",
		"POSTGRES_USER":       "pg_user",
		"POSTGRES_PASSWORD":   "pg_pass",
		"POSTGRES_SSLMODE":    "require",
		"REDIS_HOST":          "redis-host",
		"REDIS_PASSWORD":      "redis_pass",
		"S3_ENDPOINT":         "https://s3.example.com",
		"S3_ACCESS_KEY":       "ak",
		"S3_SECRET_KEY":       "sk",
		"AUTH_JWT_SECRET":     "super-secret",
		"PLUGIN_DIR":          "/opt/plugins",
		"CUSTOM_PLUGIN_DIR":   "/opt/custom",
		"AI_ENABLED":          "true",
		"AI_DEFAULT_PROVIDER": "ollama",
		"AI_OPENAI_API_KEY":   "sk-test",
		"AI_OLLAMA_BASE_URL":  "http://localhost:11434",
		"CORS_ORIGINS":        "https://a.com,https://b.com",
	}
	for k, v := range envVars {
		t.Setenv(k, v)
	}

	cfg := DefaultConfig()
	cfg.LoadFromEnv()

	if cfg.Server.Port != "9090" {
		t.Errorf("Server.Port = %s, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %s, want 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Server.Env != "production" {
		t.Errorf("Server.Env = %s, want production", cfg.Server.Env)
	}
	if cfg.Server.LogLevel != "debug" {
		t.Errorf("Server.LogLevel = %s, want debug", cfg.Server.LogLevel)
	}
	if cfg.Database.Host != "pg-host" {
		t.Errorf("Database.Host = %s, want pg-host", cfg.Database.Host)
	}
	if cfg.Database.DBName != "prod_db" {
		t.Errorf("Database.DBName = %s, want prod_db", cfg.Database.DBName)
	}
	if cfg.Database.User != "pg_user" {
		t.Errorf("Database.User = %s, want pg_user", cfg.Database.User)
	}
	if cfg.Database.Password != "pg_pass" {
		t.Errorf("Database.Password = %s, want pg_pass", cfg.Database.Password)
	}
	if cfg.Database.SSLMode != "require" {
		t.Errorf("Database.SSLMode = %s, want require", cfg.Database.SSLMode)
	}
	if cfg.Redis.Host != "redis-host" {
		t.Errorf("Redis.Host = %s, want redis-host", cfg.Redis.Host)
	}
	if cfg.Redis.Password != "redis_pass" {
		t.Errorf("Redis.Password = %s, want redis_pass", cfg.Redis.Password)
	}
	if cfg.S3.Endpoint != "https://s3.example.com" {
		t.Errorf("S3.Endpoint = %s, want https://s3.example.com", cfg.S3.Endpoint)
	}
	if cfg.S3.AccessKey != "ak" {
		t.Errorf("S3.AccessKey = %s, want ak", cfg.S3.AccessKey)
	}
	if cfg.S3.SecretKey != "sk" {
		t.Errorf("S3.SecretKey = %s, want sk", cfg.S3.SecretKey)
	}
	if cfg.Auth.JWTSecret != "super-secret" {
		t.Errorf("Auth.JWTSecret = %s, want super-secret", cfg.Auth.JWTSecret)
	}
	if cfg.Plugin.Dir != "/opt/plugins" {
		t.Errorf("Plugin.Dir = %s, want /opt/plugins", cfg.Plugin.Dir)
	}
	if cfg.Plugin.CustomDir != "/opt/custom" {
		t.Errorf("Plugin.CustomDir = %s, want /opt/custom", cfg.Plugin.CustomDir)
	}
	if !cfg.AI.Enabled {
		t.Error("AI.Enabled should be true")
	}
	if cfg.AI.DefaultProvider != "ollama" {
		t.Errorf("AI.DefaultProvider = %s, want ollama", cfg.AI.DefaultProvider)
	}
	if cfg.AI.OpenAIAPIKey != "sk-test" {
		t.Errorf("AI.OpenAIAPIKey = %s, want sk-test", cfg.AI.OpenAIAPIKey)
	}
	if cfg.AI.OllamaBaseURL != "http://localhost:11434" {
		t.Errorf("AI.OllamaBaseURL = %s, want http://localhost:11434", cfg.AI.OllamaBaseURL)
	}
	if len(cfg.CORS.Origins) != 2 {
		t.Fatalf("CORS.Origins length = %d, want 2", len(cfg.CORS.Origins))
	}
	if cfg.CORS.Origins[0] != "https://a.com" || cfg.CORS.Origins[1] != "https://b.com" {
		t.Errorf("CORS.Origins = %v, want [https://a.com, https://b.com]", cfg.CORS.Origins)
	}
}

func TestLoadFromEnv_EmptyVarsIgnored(t *testing.T) {
	cfg := DefaultConfig()
	originalPort := cfg.Server.Port

	// Ensure the env var is not set
	os.Unsetenv("SERVER_PORT")
	cfg.LoadFromEnv()

	if cfg.Server.Port != originalPort {
		t.Errorf("empty env var should not override default, got %s", cfg.Server.Port)
	}
}

func TestLoadFromEnv_AIEnabledFalse(t *testing.T) {
	t.Setenv("AI_ENABLED", "false")
	cfg := DefaultConfig()
	cfg.LoadFromEnv()
	if cfg.AI.Enabled {
		t.Error("AI_ENABLED=false should not enable AI")
	}
}

func TestValidate_InsecureDefaults(t *testing.T) {
	cfg := DefaultConfig()
	warnings := cfg.Validate()

	// Default config should produce warnings for insecure defaults
	if len(warnings) == 0 {
		t.Fatal("expected warnings for insecure defaults, got none")
	}

	foundJWT := false
	foundDB := false
	foundRedis := false
	for _, w := range warnings {
		if containsStr(w, "auth.jwt_secret") {
			foundJWT = true
		}
		if containsStr(w, "database.password") {
			foundDB = true
		}
		if containsStr(w, "redis.password") {
			foundRedis = true
		}
	}
	if !foundJWT {
		t.Error("expected warning for auth.jwt_secret")
	}
	if !foundDB {
		t.Error("expected warning for database.password")
	}
	if !foundRedis {
		t.Error("expected warning for redis.password")
	}
}

func TestValidate_SecureConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Auth.JWTSecret = "production-secret-key"
	cfg.Database.Password = "strong-db-pass"
	cfg.Redis.Password = "strong-redis-pass"

	warnings := cfg.Validate()
	for _, w := range warnings {
		if containsStr(w, "INSECURE DEFAULT") {
			t.Errorf("unexpected insecure default warning: %s", w)
		}
	}
}

func TestValidate_ListenAllInterfaces(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Host = "0.0.0.0"
	cfg.Auth.JWTSecret = "secure"
	cfg.Database.Password = "secure"
	cfg.Redis.Password = "secure"

	warnings := cfg.Validate()
	found := false
	for _, w := range warnings {
		if containsStr(w, "0.0.0.0") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about 0.0.0.0 listen address")
	}
}

func TestValidate_WildcardCORS(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CORS.Origins = []string{"*"}
	cfg.Auth.JWTSecret = "secure"
	cfg.Database.Password = "secure"
	cfg.Redis.Password = "secure"

	warnings := cfg.Validate()
	found := false
	for _, w := range warnings {
		if containsStr(w, "cors.origins") {
			found = true
		}
	}
	if !found {
		t.Error("expected warning about wildcard CORS origins")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
