package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/database"
)

// ScanIgnore represents a CVE to ignore for a scan target.
type ScanIgnore struct {
	ID        uuid.UUID  `json:"id"`
	TenantID  uuid.UUID  `json:"tenant_id"`
	TargetID  uuid.UUID  `json:"target_id"`
	CveID     string     `json:"cve_id"`
	Reason    *string    `json:"reason,omitempty"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	// Joined from scan_targets
	TargetName string `json:"target_name,omitempty"`
	TargetRef  string `json:"target_ref,omitempty"`
}

// ScanIgnoreRepository provides data access for scan ignores.
type ScanIgnoreRepository struct {
	db *database.DB
}

// NewScanIgnoreRepository creates a new ScanIgnoreRepository.
func NewScanIgnoreRepository(db *database.DB) *ScanIgnoreRepository {
	return &ScanIgnoreRepository{db: db}
}

// List returns all scan ignores for a tenant.
func (r *ScanIgnoreRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*ScanIgnore, error) {
	query := `
		SELECT si.id, si.tenant_id, si.target_id, si.cve_id, si.reason, si.created_by, si.created_at,
		       COALESCE(st.name, '') as target_name, COALESCE(st.target_ref, '') as target_ref
		FROM scan_ignores si
		LEFT JOIN scan_targets st ON st.id = si.target_id
		WHERE si.tenant_id = $1
		ORDER BY si.created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list scan ignores: %w", err)
	}
	defer rows.Close()

	var ignores []*ScanIgnore
	for rows.Next() {
		ignore := &ScanIgnore{}
		if err := rows.Scan(
			&ignore.ID, &ignore.TenantID, &ignore.TargetID, &ignore.CveID,
			&ignore.Reason, &ignore.CreatedBy, &ignore.CreatedAt,
			&ignore.TargetName, &ignore.TargetRef,
		); err != nil {
			return nil, fmt.Errorf("scan ignore row: %w", err)
		}
		ignores = append(ignores, ignore)
	}

	return ignores, nil
}

// ListByTarget returns all scan ignores for a specific target.
func (r *ScanIgnoreRepository) ListByTarget(ctx context.Context, targetID, tenantID uuid.UUID) ([]*ScanIgnore, error) {
	query := `
		SELECT id, tenant_id, target_id, cve_id, reason, created_by, created_at
		FROM scan_ignores
		WHERE target_id = $1 AND tenant_id = $2
		ORDER BY created_at DESC
	`

	rows, err := r.db.Pool.Query(ctx, query, targetID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list scan ignores by target: %w", err)
	}
	defer rows.Close()

	var ignores []*ScanIgnore
	for rows.Next() {
		ignore := &ScanIgnore{}
		if err := rows.Scan(
			&ignore.ID, &ignore.TenantID, &ignore.TargetID, &ignore.CveID,
			&ignore.Reason, &ignore.CreatedBy, &ignore.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ignore row: %w", err)
		}
		ignores = append(ignores, ignore)
	}

	return ignores, nil
}

// Create adds a new scan ignore.
func (r *ScanIgnoreRepository) Create(ctx context.Context, ignore *ScanIgnore) error {
	query := `
		INSERT INTO scan_ignores (id, tenant_id, target_id, cve_id, reason, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at
	`

	return r.db.Pool.QueryRow(
		ctx, query,
		ignore.ID, ignore.TenantID, ignore.TargetID, ignore.CveID,
		ignore.Reason, ignore.CreatedBy,
	).Scan(&ignore.CreatedAt)
}

// Delete removes a scan ignore.
func (r *ScanIgnoreRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) error {
	query := `DELETE FROM scan_ignores WHERE id = $1 AND tenant_id = $2`
	_, err := r.db.Pool.Exec(ctx, query, id, tenantID)
	return err
}

// DeleteByTargetAndCVE removes a scan ignore by target and CVE.
func (r *ScanIgnoreRepository) DeleteByTargetAndCVE(ctx context.Context, targetID uuid.UUID, cveID string, tenantID uuid.UUID) error {
	query := `DELETE FROM scan_ignores WHERE target_id = $1 AND cve_id = $2 AND tenant_id = $3`
	_, err := r.db.Pool.Exec(ctx, query, targetID, cveID, tenantID)
	return err
}

// GetIgnoreFileContent generates .trivyignore file content for a target.
func (r *ScanIgnoreRepository) GetIgnoreFileContent(ctx context.Context, targetID, tenantID uuid.UUID) (string, error) {
	ignores, err := r.ListByTarget(ctx, targetID, tenantID)
	if err != nil {
		return "", err
	}

	if len(ignores) == 0 {
		return "", nil
	}

	content := "# Auto-generated .trivyignore\n"
	content += "# Ignored CVEs for this scan target\n\n"
	for _, ignore := range ignores {
		if ignore.Reason != nil && *ignore.Reason != "" {
			content += fmt.Sprintf("# %s\n", *ignore.Reason)
		}
		content += fmt.Sprintf("%s\n", ignore.CveID)
	}

	return content, nil
}
