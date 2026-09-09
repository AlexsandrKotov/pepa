package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/database"
)

// ListOptions provides common pagination and filtering parameters.
type ListOptions struct {
	Limit  int
	Offset int
	Search string
}

// BaseRepository provides common CRUD operations for entities that follow
// the standard pattern: list with pagination, get by ID, create, update, delete.
// Specific repositories can embed this and add custom queries.
type BaseRepository struct {
	DB    *database.DB
	Table string
}

// NewBaseRepository creates a new base repository for the given table.
func NewBaseRepository(db *database.DB, table string) BaseRepository {
	return BaseRepository{DB: db, Table: table}
}

// Count returns the total number of rows for the tenant.
func (r *BaseRepository) Count(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	err := r.DB.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE tenant_id = $1", r.Table),
		tenantID,
	).Scan(&count)
	return count, err
}

// Delete removes a row by ID and tenant. Returns true if a row was deleted.
func (r *BaseRepository) Delete(ctx context.Context, id, tenantID uuid.UUID) (bool, error) {
	tag, err := r.DB.Exec(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE id = $1 AND tenant_id = $2", r.Table),
		id, tenantID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// Exists checks whether a row with the given ID exists for the tenant.
func (r *BaseRepository) Exists(ctx context.Context, id, tenantID uuid.UUID) (bool, error) {
	var exists bool
	err := r.DB.QueryRow(ctx,
		fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE id = $1 AND tenant_id = $2)", r.Table),
		id, tenantID,
	).Scan(&exists)
	return exists, err
}
