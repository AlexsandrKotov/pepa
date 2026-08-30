-- Migration: Add indexes for LDAP and Azure AD authentication
-- Supports faster lookups by auth_provider and external_id

CREATE INDEX IF NOT EXISTS idx_users_auth_provider ON users(auth_provider) WHERE auth_provider IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_users_external_id ON users(external_id) WHERE external_id IS NOT NULL;
