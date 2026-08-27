package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
)

// ClusterRepository handles cluster persistence.
type ClusterRepository struct {
	pool *pgxpool.Pool
}

// NewClusterRepository creates a new cluster repository.
func NewClusterRepository(db *database.DB) *ClusterRepository {
	return &ClusterRepository{pool: db.Pool}
}

// Cluster represents a Kubernetes cluster.
type Cluster struct {
	ID                uuid.UUID         `json:"id"`
	TenantID          uuid.UUID         `json:"tenant_id"`
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	Environment       string            `json:"environment"`
	APIServerURL      string            `json:"api_server_url"`
	FluxInstalled     bool              `json:"flux_installed"`
	Status            string            `json:"status"`
	NodeCount         int               `json:"node_count"`
	KubernetesVersion string            `json:"kubernetes_version"`
	Labels            map[string]string `json:"labels"`
	Notes             string            `json:"notes"`
	IsActive          bool              `json:"is_active"`
	HasKubeconfig     bool              `json:"has_kubeconfig"`
	ConnectionID      *uuid.UUID        `json:"connection_id,omitempty"`
	LastHeartbeatAt   *time.Time        `json:"last_heartbeat_at,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

// FluxResource represents a FluxCD CRD (Kustomization/HelmRelease).
type FluxResource struct {
	ID               uuid.UUID  `json:"id"`
	TenantID         uuid.UUID  `json:"tenant_id"`
	ClusterID        uuid.UUID  `json:"cluster_id"`
	Namespace        string     `json:"namespace"`
	Name             string     `json:"name"`
	Kind             string     `json:"kind"`
	Status           string     `json:"status"`
	Message          string     `json:"message,omitempty"`
	Revision         string     `json:"revision,omitempty"`
	LastReconciledAt *time.Time `json:"last_reconciled_at,omitempty"`
	Suspended        bool       `json:"suspended"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// List returns all clusters for a tenant.
func (r *ClusterRepository) List(ctx context.Context, tenantID uuid.UUID) ([]Cluster, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), environment, COALESCE(api_server_url,''),
		       flux_installed, status, node_count, COALESCE(kubernetes_version,''),
		       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
		       is_active, (kubeconfig_encrypted IS NOT NULL AND kubeconfig_encrypted != ''),
		       connection_id, last_heartbeat_at, created_at, updated_at
		FROM clusters WHERE tenant_id = $1
		ORDER BY name ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	items := make([]Cluster, 0)
	for rows.Next() {
		var c Cluster
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Name, &c.Description, &c.Environment, &c.APIServerURL,
			&c.FluxInstalled, &c.Status, &c.NodeCount, &c.KubernetesVersion,
			&c.Labels, &c.Notes,
			&c.IsActive, &c.HasKubeconfig, &c.ConnectionID, &c.LastHeartbeatAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cluster: %w", err)
		}
		items = append(items, c)
	}
	return items, nil
}

// Get returns a cluster by ID, scoped to tenantID (zero = no filter).
func (r *ClusterRepository) Get(ctx context.Context, id, tenantID uuid.UUID) (*Cluster, error) {
	query := `
		SELECT id, tenant_id, name, COALESCE(description,''), environment, COALESCE(api_server_url,''),
		       flux_installed, status, node_count, COALESCE(kubernetes_version,''),
		       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
		       is_active, (kubeconfig_encrypted IS NOT NULL AND kubeconfig_encrypted != ''),
		       connection_id, last_heartbeat_at, created_at, updated_at
		FROM clusters WHERE id = $1`
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	row := r.pool.QueryRow(ctx, query, args...)

	var c Cluster
	if err := row.Scan(&c.ID, &c.TenantID, &c.Name, &c.Description, &c.Environment, &c.APIServerURL,
		&c.FluxInstalled, &c.Status, &c.NodeCount, &c.KubernetesVersion,
		&c.Labels, &c.Notes,
		&c.IsActive, &c.HasKubeconfig, &c.ConnectionID, &c.LastHeartbeatAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("cluster not found: %s", id)
		}
		return nil, fmt.Errorf("get cluster: %w", err)
	}
	return &c, nil
}

// Create inserts a new cluster.
func (r *ClusterRepository) Create(ctx context.Context, c *Cluster) error {
	c.ID = uuid.New()
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now

	labelsJSON, _ := json.Marshal(c.Labels)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO clusters (id, tenant_id, name, description, environment, api_server_url,
		                      flux_installed, status, node_count, kubernetes_version,
		                      labels, notes, is_active, connection_id, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`, c.ID, c.TenantID, c.Name, c.Description, c.Environment, c.APIServerURL,
		c.FluxInstalled, c.Status, c.NodeCount, c.KubernetesVersion,
		labelsJSON, c.Notes, c.IsActive, c.ConnectionID, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create cluster: %w", err)
	}
	return nil
}

// Update modifies an existing cluster.
func (r *ClusterRepository) Update(ctx context.Context, c *Cluster) error {
	c.UpdatedAt = time.Now().UTC()
	labelsJSON, _ := json.Marshal(c.Labels)
	_, err := r.pool.Exec(ctx, `
		UPDATE clusters SET name=$2, description=$3, environment=$4, api_server_url=$5,
		       flux_installed=$6, status=$7, node_count=$8, kubernetes_version=$9,
		       labels=$10, notes=$11, is_active=$12, connection_id=$13, updated_at=$14
		WHERE id=$1
	`, c.ID, c.Name, c.Description, c.Environment, c.APIServerURL,
		c.FluxInstalled, c.Status, c.NodeCount, c.KubernetesVersion,
		labelsJSON, c.Notes, c.IsActive, c.ConnectionID, c.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update cluster: %w", err)
	}
	return nil
}

// Delete removes a cluster and all dependent rows in a transaction.
func (r *ClusterRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Delete dependent rows that reference this cluster via FK
	dependents := []struct{ table, column string }{
		{"service_deployments", "cluster_id"},
		{"deployments", "target_cluster_id"},
	}
	for _, d := range dependents {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s = $1`, d.table, d.column), id); err != nil {
			return fmt.Errorf("delete dependents from %s: %w", d.table, err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete cluster: %w", err)
	}

	return tx.Commit(ctx)
}

// FindByConnectionID returns the cluster linked to a connection, if any.
func (r *ClusterRepository) FindByConnectionID(ctx context.Context, connectionID uuid.UUID) (*Cluster, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, COALESCE(description,''), environment, COALESCE(api_server_url,''),
		       flux_installed, status, node_count, COALESCE(kubernetes_version,''),
		       COALESCE(labels,'{}'::jsonb), COALESCE(notes,''),
		       is_active, (kubeconfig_encrypted IS NOT NULL AND kubeconfig_encrypted != ''),
		       connection_id, last_heartbeat_at, created_at, updated_at
		FROM clusters WHERE connection_id = $1
	`, connectionID)

	var c Cluster
	if err := row.Scan(&c.ID, &c.TenantID, &c.Name, &c.Description, &c.Environment, &c.APIServerURL,
		&c.FluxInstalled, &c.Status, &c.NodeCount, &c.KubernetesVersion,
		&c.Labels, &c.Notes,
		&c.IsActive, &c.HasKubeconfig, &c.ConnectionID, &c.LastHeartbeatAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // not found — not an error
		}
		return nil, fmt.Errorf("find cluster by connection: %w", err)
	}
	return &c, nil
}

// DeleteByConnectionID removes the cluster linked to a connection.
func (r *ClusterRepository) DeleteByConnectionID(ctx context.Context, connectionID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM clusters WHERE connection_id = $1`, connectionID)
	if err != nil {
		return fmt.Errorf("delete cluster by connection: %w", err)
	}
	return nil
}

// SaveKubeconfig stores the kubeconfig for a cluster (encrypted).
func (r *ClusterRepository) SaveKubeconfig(ctx context.Context, id uuid.UUID, kubeconfig string) error {
	// Encrypt the kubeconfig before storage
	encrypted, err := crypto.Encrypt(kubeconfig)
	if err != nil {
		return fmt.Errorf("encrypt kubeconfig: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE clusters SET kubeconfig_encrypted=$2, status='connected', updated_at=NOW()
		WHERE id=$1
	`, id, encrypted)
	if err != nil {
		return fmt.Errorf("save kubeconfig: %w", err)
	}
	return nil
}

// GetKubeconfig retrieves and decrypts the kubeconfig for a cluster, scoped to
// tenantID (zero = no filter).
func (r *ClusterRepository) GetKubeconfig(ctx context.Context, id, tenantID uuid.UUID) (string, error) {
	var encrypted string
	query := `SELECT COALESCE(kubeconfig_encrypted, '') FROM clusters WHERE id = $1`
	args := []interface{}{id}
	if tenantID != uuid.Nil {
		query += " AND tenant_id = $2"
		args = append(args, tenantID)
	}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&encrypted)
	if err != nil {
		return "", fmt.Errorf("get kubeconfig: %w", err)
	}
	if encrypted == "" {
		return "", nil
	}
	// If still encrypted, attempt decryption
	kubeconfig, err := crypto.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt kubeconfig: %w (encryption key may have changed — set ENCRYPTION_KEY or AUTH_JWT_SECRET to the original value)", err)
	}
	return kubeconfig, nil
}

// ListFluxResources returns FluxCD resources for a cluster.
func (r *ClusterRepository) ListFluxResources(ctx context.Context, clusterID uuid.UUID) ([]FluxResource, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, cluster_id, namespace, name, kind, status,
		       COALESCE(message,''), COALESCE(revision,''), last_reconciled_at,
		       suspended, created_at, updated_at
		FROM flux_resources WHERE cluster_id = $1
		ORDER BY namespace, name
	`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("query flux resources: %w", err)
	}
	defer rows.Close()

	items := make([]FluxResource, 0)
	for rows.Next() {
		var f FluxResource
		if err := rows.Scan(&f.ID, &f.TenantID, &f.ClusterID, &f.Namespace, &f.Name,
			&f.Kind, &f.Status, &f.Message, &f.Revision, &f.LastReconciledAt,
			&f.Suspended, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan flux resource: %w", err)
		}
		items = append(items, f)
	}
	return items, nil
}

// UpsertFluxResource creates or updates a FluxCD resource.
func (r *ClusterRepository) UpsertFluxResource(ctx context.Context, f *FluxResource) error {
	f.ID = uuid.New()
	now := time.Now().UTC()
	f.CreatedAt = now
	f.UpdatedAt = now

	_, err := r.pool.Exec(ctx, `
		INSERT INTO flux_resources (id, tenant_id, cluster_id, namespace, name, kind, status, message, revision, suspended, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (cluster_id, namespace, name, kind) DO UPDATE SET
			status = EXCLUDED.status, message = EXCLUDED.message,
			revision = EXCLUDED.revision, last_reconciled_at = EXCLUDED.last_reconciled_at,
			suspended = EXCLUDED.suspended, updated_at = EXCLUDED.updated_at
	`, f.ID, f.TenantID, f.ClusterID, f.Namespace, f.Name, f.Kind,
		f.Status, f.Message, f.Revision, f.Suspended, f.CreatedAt, f.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert flux resource: %w", err)
	}
	return nil
}
