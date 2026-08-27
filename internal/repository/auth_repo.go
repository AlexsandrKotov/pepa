package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User represents a user in the system.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	IsActive     bool
	LastLogin    *time.Time
	TokenVersion int
	CreatedAt    time.Time
}

// AuthRepository handles database operations for authentication.
type AuthRepository struct {
	pool *pgxpool.Pool
}

// NewAuthRepository creates a new AuthRepository.
func NewAuthRepository(pool *pgxpool.Pool) *AuthRepository {
	return &AuthRepository{pool: pool}
}

// GetUserByEmail returns a user by email.
func (r *AuthRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, name, COALESCE(password_hash, ''), is_active, last_login_at, COALESCE(token_version, 0), created_at
		FROM users WHERE email = $1
	`, email).Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.IsActive, &user.LastLogin, &user.TokenVersion, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &user, nil
}

// GetUserByID returns a user by ID.
func (r *AuthRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var user User
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, name, COALESCE(password_hash, ''), is_active, last_login_at, COALESCE(token_version, 0), created_at
		FROM users WHERE id = $1
	`, id).Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.IsActive, &user.LastLogin, &user.TokenVersion, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}

// UpdateLastLogin updates the last_login_at timestamp for a user.
func (r *AuthRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("update last login: %w", err)
	}
	return nil
}

// UpdatePassword updates a user's password hash and clears must_change_password.
func (r *AuthRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET password_hash = $2, must_change_password = false, updated_at = NOW()
		WHERE id = $1
	`, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// GetTokenVersion returns the token version for a user.
func (r *AuthRepository) GetTokenVersion(ctx context.Context, userID uuid.UUID) (int, error) {
	var version int
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(token_version, 0) FROM users WHERE id = $1`, userID).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get token version: %w", err)
	}
	return version, nil
}

// IncrementTokenVersion increments the token version for a user (invalidates all existing tokens).
func (r *AuthRepository) IncrementTokenVersion(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET token_version = token_version + 1 WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("increment token version: %w", err)
	}
	return nil
}

// GetUserPermissions returns all permissions for a user in a tenant.
func (r *AuthRepository) GetUserPermissions(ctx context.Context, tenantID, userID uuid.UUID) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT p.resource || ':' || p.action
		FROM permissions p
		JOIN role_assignments ra ON ra.role_id = p.role_id
		WHERE ra.tenant_id = $1
		  AND ra.user_id = $2
		  AND ra.is_active = true
		  AND p.effect = 'allow'
	`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("get user permissions: %w", err)
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var perm string
		if err := rows.Scan(&perm); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		permissions = append(permissions, perm)
	}
	return permissions, nil
}

// GetMustChangePassword returns whether a user must change their password.
func (r *AuthRepository) GetMustChangePassword(ctx context.Context, userID uuid.UUID) (bool, error) {
	var mustChange bool
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(must_change_password, false) FROM users WHERE id = $1`, userID).Scan(&mustChange)
	if err != nil {
		return false, fmt.Errorf("get must change password: %w", err)
	}
	return mustChange, nil
}

// GetUserAuthInfo returns authentication-related user info (is_active, token_version, email).
func (r *AuthRepository) GetUserAuthInfo(ctx context.Context, userID uuid.UUID) (isActive bool, tokenVersion int, email string, err error) {
	err = r.pool.QueryRow(ctx, `
		SELECT is_active, COALESCE(token_version, 0), email FROM users WHERE id = $1
	`, userID).Scan(&isActive, &tokenVersion, &email)
	if err != nil {
		return false, 0, "", fmt.Errorf("get user auth info: %w", err)
	}
	return isActive, tokenVersion, email, nil
}

// UserExistsByEmail checks if a user with the given email exists.
func (r *AuthRepository) UserExistsByEmail(ctx context.Context, email string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) > 0 FROM users WHERE email = $1`, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check user exists: %w", err)
	}
	return exists, nil
}

// GetUserIDByEmail returns the user ID for a given email.
func (r *AuthRepository) GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("get user id by email: %w", err)
	}
	return userID, nil
}

// DeactivateUser deactivates a user account.
func (r *AuthRepository) DeactivateUser(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET is_active = false, updated_at = NOW() WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("deactivate user: %w", err)
	}
	return nil
}

// CreateUser creates a new user account.
func (r *AuthRepository) CreateUser(ctx context.Context, userID uuid.UUID, email, name, passwordHash string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, true, NOW(), NOW())
	`, userID, email, name, passwordHash)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// UpdateUser updates user fields.
func (r *AuthRepository) UpdateUser(ctx context.Context, userID uuid.UUID, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}

	setClauses := []string{"updated_at = NOW()"}
	args := []interface{}{}
	argIdx := 1

	for key, value := range fields {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, argIdx))
		args = append(args, value)
		argIdx++
	}

	args = append(args, userID)
	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d",
		setClauses[0], argIdx)
	for i := 1; i < len(setClauses); i++ {
		query = fmt.Sprintf("UPDATE users SET %s, %s WHERE id = $%d",
			query[len("UPDATE users SET "):len(query)-len(fmt.Sprintf(" WHERE id = $%d", argIdx))],
			setClauses[i], argIdx)
	}

	_, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// ListUsers returns a list of users with optional filtering.
func (r *AuthRepository) ListUsers(ctx context.Context, search string, isActive *bool) ([]User, error) {
	query := `
		SELECT id, email, name, COALESCE(password_hash, ''), is_active, last_login_at, COALESCE(token_version, 0), created_at
		FROM users
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	if search != "" {
		query += fmt.Sprintf(" AND (email ILIKE $%d OR name ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	if isActive != nil {
		query += fmt.Sprintf(" AND is_active = $%d", argIdx)
		args = append(args, *isActive)
	}

	query += " ORDER BY name ASC"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.IsActive, &user.LastLogin, &user.TokenVersion, &user.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, user)
	}
	return users, nil
}

// ResetUserPassword resets a user's password.
func (r *AuthRepository) ResetUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("reset user password: %w", err)
	}
	return nil
}
