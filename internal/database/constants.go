// Package database provides constants for well-known UUIDs used across
// the PEPA platform. Centralizing these values avoids scattering magic
// strings throughout the codebase and SQL migrations.
package database

// Default well-known UUIDs used for seeding and referencing across the platform.
// These match the values inserted by the SQL migrations and init-db.sql.
const (
	DefaultOrganizationID = "00000000-0000-0000-0000-000000000001"
	DefaultTenantID       = "00000000-0000-0000-0000-000000000002"

	// Default scorecard: Production Readiness
	DefaultScorecardID = "10000000-0000-0000-0000-000000000001"

	// Default super admin user (seeded in migration 017)
	SuperAdminUserID = "00000000-0000-0000-0000-000000000010"

	// System RBAC role IDs
	AdminRoleID     = "20000000-0000-0000-0000-000000000001"
	DeveloperRoleID = "20000000-0000-0000-0000-000000000002"
	ViewerRoleID    = "20000000-0000-0000-0000-000000000003"
)
