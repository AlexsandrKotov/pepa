package auth

import "context"

// UserInfo is a normalized user identity returned by any auth provider.
// Each provider (OIDC, Azure AD, LDAP) maps its native claims to this struct.
type UserInfo struct {
	Email      string
	Name       string
	ExternalID string // Stable external identifier (OIDC sub, Azure oid, LDAP DN)
	Groups     []string
}

// AuthProvider is the common interface for all authentication backends.
type AuthProvider interface {
	// Name returns the provider identifier (e.g. "oidc", "azure", "ldap").
	Name() string
}

// ExternalUserInfo is implemented by providers that authenticate external
// users via redirect (OIDC/Azure) or direct bind (LDAP).
type ExternalUserInfo interface {
	AuthProvider
	// FetchUserInfo retrieves user identity after successful authentication.
	// For OIDC/Azure, accessToken is the token from the IdP.
	// For LDAP, this is not used — authentication returns user info directly.
	FetchUserInfo(ctx context.Context, accessToken string) (*UserInfo, error)
}
