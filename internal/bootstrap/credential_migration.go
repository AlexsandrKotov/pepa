package bootstrap

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/pepa/pepa/pkg/utils"
)

// MigrateEncryptCredentials encrypts any plain text credentials in helm_repositories,
// connections, clusters, docker_hosts, and plugins.
// This runs on startup to migrate existing data to encrypted storage.
func (c *Components) MigrateEncryptCredentials() {
	ctx := context.Background()

	c.migrateHelmRepoCredentials(ctx)
	c.migrateConnectionCredentials(ctx)
	c.migrateClusterCredentials(ctx)
	c.migrateDockerHostCredentials(ctx)
	c.migratePluginCredentials(ctx)
}

func (c *Components) migrateHelmRepoCredentials(ctx context.Context) {
	rows, err := c.DB.Pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(password,''), COALESCE(token,''), COALESCE(ssh_key,'')
		FROM helm_repositories
	`)
	if err != nil {
		slog.Warn("failed to list helm repos for migration", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var name, password, token, sshKey string
		if err := rows.Scan(&id, new(uuid.UUID), &name, &password, &token, &sshKey); err != nil {
			continue
		}
		if !needsEncryption(password, token, sshKey) {
			continue
		}
		repo, err := c.HelmRepo.Get(ctx, id, uuid.Nil)
		if err != nil {
			continue
		}
		if err := c.HelmRepo.Update(ctx, repo); err != nil {
			slog.Warn("failed to encrypt helm repo credentials", "repo", name, "error", err)
		} else {
			slog.Info("encrypted credentials for helm repo", "repo", name)
		}
	}
}

func (c *Components) migrateConnectionCredentials(ctx context.Context) {
	rows, err := c.DB.Pool.Query(ctx, `
		SELECT id, name, tenant_id, COALESCE(config,'{}'::jsonb)
		FROM connections
	`)
	if err != nil {
		slog.Warn("failed to list connections for migration", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var name string
		var tenantID uuid.UUID
		var configJSON []byte
		if err := rows.Scan(&id, &name, &tenantID, &configJSON); err != nil {
			continue
		}
		var config map[string]any
		if err := json.Unmarshal(configJSON, &config); err != nil {
			slog.Warn("failed to parse connection config", "connection", name, "error", err)
			continue
		}
		needsUpdate := false
		for k, v := range config {
			if strVal, ok := v.(string); ok && isSensitiveKey(k) && strVal != "" && !isEncrypted(strVal) {
				needsUpdate = true
				break
			}
		}
		if !needsUpdate {
			continue
		}
		conn, err := c.ConnectionRepo.Get(ctx, id, tenantID)
		if err != nil {
			continue
		}
		if err := c.ConnectionRepo.Update(ctx, conn); err != nil {
			slog.Warn("failed to encrypt connection credentials", "connection", name, "error", err)
		} else {
			slog.Info("encrypted credentials for connection", "connection", name)
		}
	}
}

func (c *Components) migrateClusterCredentials(ctx context.Context) {
	rows, err := c.DB.Pool.Query(ctx, `
		SELECT id, name, COALESCE(kubeconfig_encrypted,'')
		FROM clusters
	`)
	if err != nil {
		slog.Warn("failed to list clusters for migration", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var name, kubeconfig string
		if err := rows.Scan(&id, &name, &kubeconfig); err != nil {
			continue
		}
		if kubeconfig == "" || isEncrypted(kubeconfig) {
			continue
		}
		if err := c.ClusterRepo.SaveKubeconfig(ctx, id, kubeconfig); err != nil {
			slog.Warn("failed to encrypt cluster kubeconfig", "cluster", name, "error", err)
		} else {
			slog.Info("encrypted kubeconfig for cluster", "cluster", name)
		}
	}
}

func (c *Components) migrateDockerHostCredentials(ctx context.Context) {
	rows, err := c.DB.Pool.Query(ctx, `
		SELECT id, name, tenant_id, COALESCE(tls_ca_cert,''), COALESCE(tls_cert,''), COALESCE(tls_key,''), COALESCE(ssh_key,'')
		FROM docker_hosts
	`)
	if err != nil {
		slog.Warn("failed to list docker hosts for migration", "error", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var name string
		var tenantID uuid.UUID
		var tlsCA, tlsCert, tlsKey, sshKey string
		if err := rows.Scan(&id, &name, &tenantID, &tlsCA, &tlsCert, &tlsKey, &sshKey); err != nil {
			continue
		}
		if !needsEncryption(tlsCA, tlsCert, tlsKey, sshKey) {
			continue
		}
		host, err := c.DockerHostRepo.GetHost(ctx, id, tenantID)
		if err != nil {
			continue
		}
		if err := c.DockerHostRepo.UpdateHost(ctx, host); err != nil {
			slog.Warn("failed to encrypt docker host credentials", "host", name, "error", err)
		} else {
			slog.Info("encrypted credentials for docker host", "host", name)
		}
	}
}

func (c *Components) migratePluginCredentials(ctx context.Context) {
	rows, err := c.DB.Pool.Query(ctx, `
		SELECT id, name, COALESCE(config,'{}'::jsonb)
		FROM plugins
	`)
	if err != nil {
		slog.Warn("failed to list plugins for migration", "error", err)
		return
	}
	defer rows.Close()

	pluginSensitiveKeys := []string{"token", "password", "api_key", "api_token", "secret", "access_token", "private_token"}

	for rows.Next() {
		var id uuid.UUID
		var name string
		var configJSON []byte
		if err := rows.Scan(&id, &name, &configJSON); err != nil {
			continue
		}
		var config map[string]any
		if err := json.Unmarshal(configJSON, &config); err != nil {
			slog.Warn("failed to parse plugin config", "plugin", name, "error", err)
			continue
		}
		needsUpdate := false
		for _, key := range pluginSensitiveKeys {
			if val, ok := config[key]; ok {
				if strVal, ok := val.(string); ok && strVal != "" && !isEncrypted(strVal) {
					needsUpdate = true
					break
				}
			}
		}
		if !needsUpdate {
			continue
		}
		plugin, err := c.PluginRepo.Get(ctx, id)
		if err != nil {
			continue
		}
		if err := c.PluginRepo.Register(ctx, plugin); err != nil {
			slog.Warn("failed to encrypt plugin config", "plugin", name, "error", err)
		} else {
			slog.Info("encrypted config for plugin", "plugin", name)
		}
	}
}

// isEncrypted reports whether a value has already been encrypted.
func isEncrypted(value string) bool {
	return len(value) > 4 && value[:4] == "enc:"
}

// needsEncryption returns true if any of the given values is non-empty and not yet encrypted.
func needsEncryption(values ...string) bool {
	for _, v := range values {
		if v != "" && !isEncrypted(v) {
			return true
		}
	}
	return false
}

func isSensitiveKey(key string) bool {
	return utils.IsSensitiveKey(key)
}
