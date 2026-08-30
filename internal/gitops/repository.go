// Package gitops implements GitOps manifest repository management:
// binding Git repos, scanning for FluxCD/ArgoCD resources, parsing manifests,
// editing values with Git commit, deployment tracking, and topology visualization.
package gitops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/database"
)

// Repository handles persistence for GitOps manifest repositories.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository creates a new GitOps repository store.
func NewRepository(db *database.DB) *Repository {
	return &Repository{pool: db.Pool}
}

// Repo represents a bound Git repository containing GitOps manifests.
type Repo struct {
	ID            uuid.UUID         `json:"id"`
	TenantID      uuid.UUID         `json:"tenant_id"`
	Name          string            `json:"name"`
	ConnectionID  *uuid.UUID        `json:"connection_id,omitempty"`
	RepoURL       string            `json:"repo_url"`
	Branch        string            `json:"branch"`
	Path          string            `json:"path"`
	EngineType    string            `json:"engine_type"` // fluxcd | argocd | auto
	ScanStatus    string            `json:"scan_status"` // pending | scanning | ready | error
	ScanError     string            `json:"scan_error,omitempty"`
	LastScannedAt *time.Time        `json:"last_scanned_at,omitempty"`
	Config        map[string]string `json:"config"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// Resource represents a parsed GitOps manifest resource (HelmRelease, Kustomization, Application).
type Resource struct {
	Kind       string                 `json:"kind"` // HelmRelease, Kustomization, Application, ApplicationSet, Kustomize, KubernetesManifest
	APIVersion string                 `json:"api_version"`
	Name       string                 `json:"name"`
	Namespace  string                 `json:"namespace"`
	FilePath   string                 `json:"file_path"`             // relative path in repo
	Chart      string                 `json:"chart,omitempty"`       // HelmRelease: chart name
	Version    string                 `json:"version,omitempty"`     // HelmRelease: chart version
	Repo       string                 `json:"repo,omitempty"`        // HelmRelease: HelmRepository ref
	Values     map[string]interface{} `json:"values,omitempty"`      // inline values
	ValuesFrom []ValuesReference      `json:"values_from,omitempty"` // ConfigMap/Secret refs
	Source     *ArgoSource            `json:"source,omitempty"`      // ArgoCD source (single-source)
	Sources    []ArgoSource           `json:"sources,omitempty"`     // ArgoCD multi-source
	Dest       *ArgoDest              `json:"dest,omitempty"`        // ArgoCD destination
	Labels     map[string]string      `json:"labels,omitempty"`
	DependsOn  []string               `json:"depends_on,omitempty"`
	// Raw YAML content for the editor
	RawYAML string `json:"raw_yaml,omitempty"`
	// Suspend state (FluxCD: spec.suspend)
	Suspended bool `json:"suspended,omitempty"`
	// Cluster name detected from directory structure or labels
	Cluster string `json:"cluster,omitempty"`
	// Layout metadata (from Kustomize base/overlay analysis)
	Environment string           `json:"environment,omitempty"` // prod, staging, dev
	LayoutRole  string           `json:"layout_role,omitempty"` // base, overlay, app, app-of-apps, component
	BasePath    string           `json:"base_path,omitempty"`   // path to base this overlay extends
	Images      []KustomizeImage `json:"images,omitempty"`      // image overrides from kustomize
	// FluxCD v2 chartRef
	ChartRef *FluxChartRef `json:"chart_ref,omitempty"` // v2 chartRef reference
}

// FluxChartRef represents a FluxCD v2 chartRef (replaces spec.chart.spec).
type FluxChartRef struct {
	Kind      string `json:"kind"`                // HelmChart or OCIRepository
	Name      string `json:"name"`                // chart name
	Namespace string `json:"namespace,omitempty"` // namespace of the chart reference
}

// ValuesReference points to a ConfigMap or Secret containing Helm values.
type ValuesReference struct {
	Kind       string `json:"kind"` // ConfigMap or Secret
	Name       string `json:"name"`
	ValuesKey  string `json:"values_key,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
	Optional   bool   `json:"optional,omitempty"`
}

// ArgoSource represents an ArgoCD Application source.
type ArgoSource struct {
	RepoURL        string    `json:"repo_url"`
	Path           string    `json:"path"`
	TargetRevision string    `json:"target_revision"`
	Helm           *ArgoHelm `json:"helm,omitempty"`
}

// ArgoHelm holds ArgoCD Helm-specific source settings.
type ArgoHelm struct {
	ValueFiles []string               `json:"value_files,omitempty"`
	Values     map[string]interface{} `json:"values,omitempty"`
	Parameters []ArgoHelmParameter    `json:"parameters,omitempty"`
}

// ArgoHelmParameter is a single ArgoCD Helm parameter override.
type ArgoHelmParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ArgoDest represents an ArgoCD Application destination.
type ArgoDest struct {
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	Name      string `json:"name,omitempty"`
}

// ScanResult holds the result of scanning a GitOps repository.
type ScanResult struct {
	Resources []Resource    `json:"resources"`
	Engine    string        `json:"engine"`     // detected engine: fluxcd | argocd
	FileCount int           `json:"file_count"` // total YAML files examined
	Layout    LayoutInfo    `json:"layout"`     // detected repository layout
	Tree      *FileNode     `json:"tree"`       // hierarchical file tree
	Clusters  []ClusterInfo `json:"clusters"`   // cluster hierarchy info
}

// FileNode represents a directory or file in the repository tree.
type FileNode struct {
	Name      string      `json:"name"`
	Path      string      `json:"path"`
	Type      string      `json:"type"` // dir | file | environment | cluster | subcluster
	Children  []*FileNode `json:"children,omitempty"`
	Resources []Resource  `json:"resources,omitempty"`
	Count     int         `json:"count"` // total resources in this subtree
}

// ClusterInfo represents a cluster with optional sub-clusters.
type ClusterInfo struct {
	Name          string           `json:"name"`
	Environment   string           `json:"environment"`
	SubClusters   []SubClusterInfo `json:"sub_clusters,omitempty"`
	ResourceCount int              `json:"resource_count"`
}

// SubClusterInfo represents a sub-cluster within an environment.
type SubClusterInfo struct {
	Name          string `json:"name"`
	ResourceCount int    `json:"resource_count"`
}

// ── CRUD operations ──────────────────────────────────────────────

// List returns all GitOps repos for a tenant.
func (r *Repository) List(ctx context.Context, tenantID uuid.UUID) ([]Repo, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, tenant_id, name, connection_id, repo_url, branch, path,
		       engine_type, scan_status, COALESCE(scan_error,''),
		       last_scanned_at, COALESCE(config,'{}'::jsonb),
		       created_at, updated_at
		FROM gitops_repositories WHERE tenant_id = $1
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			// Table doesn't exist (migration not yet applied) – return empty list.
			return nil, nil
		}
		return nil, fmt.Errorf("query gitops repos: %w", err)
	}
	defer rows.Close()

	var items []Repo
	for rows.Next() {
		var r Repo
		var configJSON []byte
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.ConnectionID,
			&r.RepoURL, &r.Branch, &r.Path,
			&r.EngineType, &r.ScanStatus, &r.ScanError,
			&r.LastScannedAt, &configJSON,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan gitops repo: %w", err)
		}
		r.Config = decryptConfig(parseStringMap(configJSON))
		items = append(items, r)
	}
	return items, nil
}

// Get returns a GitOps repo by ID.
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (*Repo, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, tenant_id, name, connection_id, repo_url, branch, path,
		       engine_type, scan_status, COALESCE(scan_error,''),
		       last_scanned_at, COALESCE(config,'{}'::jsonb),
		       created_at, updated_at
		FROM gitops_repositories WHERE id = $1
	`, id)

	var r2 Repo
	var configJSON []byte
	if err := row.Scan(&r2.ID, &r2.TenantID, &r2.Name, &r2.ConnectionID,
		&r2.RepoURL, &r2.Branch, &r2.Path,
		&r2.EngineType, &r2.ScanStatus, &r2.ScanError,
		&r2.LastScannedAt, &configJSON,
		&r2.CreatedAt, &r2.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("gitops repo not found: %s", id)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			return nil, fmt.Errorf("gitops repositories not configured yet: run database migrations first")
		}
		return nil, fmt.Errorf("get gitops repo: %w", err)
	}
	r2.Config = decryptConfig(parseStringMap(configJSON))
	return &r2, nil
}

// Create inserts a new GitOps repo record.
func (r *Repository) Create(ctx context.Context, r2 *Repo) error {
	r2.ID = uuid.New()
	now := time.Now().UTC()
	r2.CreatedAt = now
	r2.UpdatedAt = now
	if r2.ScanStatus == "" {
		r2.ScanStatus = "pending"
	}
	if r2.Branch == "" {
		r2.Branch = "main"
	}
	if r2.Path == "" {
		r2.Path = "."
	}
	if r2.EngineType == "" {
		r2.EngineType = "auto"
	}

	configJSON, _ := json.Marshal(encryptConfig(r2.Config))

	_, err := r.pool.Exec(ctx, `
		INSERT INTO gitops_repositories (id, tenant_id, name, connection_id, repo_url, branch, path,
		                                 engine_type, scan_status, config, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, r2.ID, r2.TenantID, r2.Name, r2.ConnectionID, r2.RepoURL, r2.Branch, r2.Path,
		r2.EngineType, r2.ScanStatus, configJSON, r2.CreatedAt, r2.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create gitops repo: %w", err)
	}
	return nil
}

// Update modifies an existing GitOps repo.
func (r *Repository) Update(ctx context.Context, r2 *Repo) error {
	r2.UpdatedAt = time.Now().UTC()
	configJSON, _ := json.Marshal(encryptConfig(r2.Config))

	_, err := r.pool.Exec(ctx, `
		UPDATE gitops_repositories
		SET name=$2, connection_id=$3, repo_url=$4, branch=$5, path=$6,
		    engine_type=$7, scan_status=$8, scan_error=$9,
		    last_scanned_at=$10, config=$11, updated_at=$12
		WHERE id=$1
	`, r2.ID, r2.Name, r2.ConnectionID, r2.RepoURL, r2.Branch, r2.Path,
		r2.EngineType, r2.ScanStatus, nullString(r2.ScanError),
		r2.LastScannedAt, configJSON, r2.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update gitops repo: %w", err)
	}
	return nil
}

// Delete removes a GitOps repo by ID.
func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM gitops_repositories WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete gitops repo: %w", err)
	}
	return nil
}

// UpdateScanStatus updates the scan status and optional error.
func (r *Repository) UpdateScanStatus(ctx context.Context, id uuid.UUID, status, scanErr string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(ctx, `
		UPDATE gitops_repositories
		SET scan_status=$2, scan_error=$3, last_scanned_at=$4, updated_at=$4
		WHERE id=$1
	`, id, status, nullString(scanErr), now)
	if err != nil {
		return fmt.Errorf("update scan status: %w", err)
	}
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────

func parseStringMap(data []byte) map[string]string {
	if len(data) == 0 {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]string{}
	}
	return m
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// encryptConfig encrypts sensitive fields (token) in the config map before storage.
func encryptConfig(cfg map[string]string) map[string]string {
	if cfg == nil {
		return cfg
	}
	if token, ok := cfg["token"]; ok && token != "" {
		encrypted, err := crypto.Encrypt(token)
		if err != nil {
			log.Printf("WARNING: failed to encrypt gitops token: %v", err)
			return cfg
		}
		cfg["token"] = encrypted
	}
	return cfg
}

// decryptConfig decrypts sensitive fields (token) in the config map after retrieval.
func decryptConfig(cfg map[string]string) map[string]string {
	if cfg == nil {
		return cfg
	}
	if token, ok := cfg["token"]; ok && token != "" {
		decrypted, err := crypto.Decrypt(token)
		if err != nil {
			log.Printf("WARNING: failed to decrypt gitops token: %v", err)
			return cfg
		}
		cfg["token"] = decrypted
	}
	return cfg
}
