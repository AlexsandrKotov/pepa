package rest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	pepacrypto "github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/repository"
)

// CredentialSource indicates where a resolved credential came from.
type CredentialSource string

const (
	CredentialSourceUser   CredentialSource = "user"
	CredentialSourceShared CredentialSource = "shared"
	CredentialSourceAdmin  CredentialSource = "admin"
)

// ResolvedCredential holds the decrypted config and metadata about where it came from.
type ResolvedCredential struct {
	// Config is the merged connection config with decrypted secrets.
	// For user/shared credentials, the sensitive fields (token, password, etc.)
	// are overridden with the user's own values.
	Config map[string]string

	// Source indicates whether this is the user's own, shared, or admin credential.
	Source CredentialSource

	// OwnerID is the user ID of the credential owner (empty for admin).
	OwnerID string
}

// ResolveConnectionCredential returns the best available credential for a
// connection using a three-tier resolution:
//
//  1. User's own user_credentials entry (matching provider + provider_url)
//  2. Shared credential via credential_shares (matching provider + provider_url)
//  3. Admin connection config (only if connection.FallbackToAdmin is true)
//
// The providerName should match the provider column in user_credentials
// (e.g. "proxmox", "vmware", "kubernetes", "jira", "gitlab").
// The providerURL is extracted from the connection config to match against.
func ResolveConnectionCredential(
	ctx context.Context,
	deps Dependencies,
	conn *repository.Connection,
	userID *uuid.UUID,
	providerName string,
	providerURLKey string, // config key that holds the provider URL (e.g. "url", "endpoint", "repo_url")
) (*ResolvedCredential, error) {
	// Build the base admin config from the connection.
	adminConfig := make(map[string]string)
	for k, v := range conn.Config {
		switch val := v.(type) {
		case string:
			adminConfig[k] = val
		case float64:
			adminConfig[k] = fmt.Sprintf("%v", val)
		case bool:
			adminConfig[k] = fmt.Sprintf("%v", val)
		default:
			adminConfig[k] = fmt.Sprintf("%v", val)
		}
	}

	providerURL := adminConfig[providerURLKey]

	// Tier 1: Try user's personal credential.
	if userID != nil && deps.Repos.UserCredential != nil && providerURL != "" {
		cred, err := deps.Repos.UserCredential.GetByProvider(ctx, *userID, providerName, providerURL)
		if err == nil && cred != nil {
			token, decErr := pepacrypto.Decrypt(cred.TokenEnc)
			if decErr == nil && token != "" {
				slog.Info("credential resolved: user personal",
					"provider", providerName, "user_id", userID.String(), "source", "user")
				merged := overrideConfigWithUserCred(adminConfig, providerName, token, cred.Username, cred.Email)
				return &ResolvedCredential{
					Config:  merged,
					Source:  CredentialSourceUser,
					OwnerID: userID.String(),
				}, nil
			}
		}

		// Tier 2: Try shared credential.
		if deps.Repos.CredentialShare != nil {
			tokenEnc, username, email, err := deps.Repos.CredentialShare.GetSharedToken(ctx, *userID, conn.TenantID, providerName, providerURL)
			if err == nil && tokenEnc != "" {
				token, decErr := pepacrypto.Decrypt(tokenEnc)
				if decErr == nil && token != "" {
					slog.Info("credential resolved: shared",
						"provider", providerName, "user_id", userID.String(), "source", "shared")
					merged := overrideConfigWithUserCred(adminConfig, providerName, token, username, email)
					return &ResolvedCredential{
						Config: merged,
						Source: CredentialSourceShared,
					}, nil
				}
			}
		}
	}

	// Tier 3: Fall back to admin connection credential.
	if conn.FallbackToAdmin {
		return &ResolvedCredential{
			Config:  adminConfig,
			Source:  CredentialSourceAdmin,
			OwnerID: "",
		}, nil
	}

	return nil, fmt.Errorf("no personal %s credential found and admin fallback is disabled for this connection", providerName)
}

// overrideConfigWithUserCred returns a copy of adminConfig with sensitive fields
// overridden by the user's personal credential values.
// The override strategy depends on the provider type.
func overrideConfigWithUserCred(adminConfig map[string]string, provider, token, username, email string) map[string]string {
	merged := make(map[string]string, len(adminConfig))
	for k, v := range adminConfig {
		merged[k] = v
	}

	switch provider {
	case "proxmox":
		// Proxmox uses token_id + token_secret. The user stores token_secret in TokenEnc
		// and token_id in Username (reuse pattern).
		merged["token_secret"] = token
		if username != "" {
			merged["token_id"] = username
		}
	case "vmware":
		// VMware uses username + password. User stores password in TokenEnc.
		merged["password"] = token
		if username != "" {
			merged["username"] = username
		}
	case "kubernetes":
		// Kubernetes uses kubeconfig. User stores their kubeconfig in TokenEnc.
		merged["kubeconfig"] = token
	case "jira":
		// Jira plugin reads 'api_token' and 'username' from config.
		// The connection stores 'url', 'username', 'password'.
		// Override both 'api_token' (plugin key) and 'password' (connection key).
		merged["api_token"] = token
		merged["password"] = token
		if username != "" {
			merged["username"] = username
		}
	case "sonarqube":
		merged["token"] = token
	case "docker":
		// Docker hosts use SSH key or TLS certs. User stores SSH key in TokenEnc.
		merged["ssh_key"] = token
	case "argocd":
		// ArgoCD uses server_url + auth_token. User stores auth_token in TokenEnc.
		merged["auth_token"] = token
		if username != "" {
			merged["username"] = username
		}
	default:
		// Generic: override the token field.
		merged["token"] = token
		if username != "" {
			merged["username"] = username
		}
	}

	if email != "" {
		merged["email"] = email
	}
	return merged
}

// FindConnectionByTypeDecrypted finds and decrypts the first connection of a given type.
func FindConnectionByTypeDecrypted(ctx context.Context, deps Dependencies, tenantID uuid.UUID, connType string) *repository.Connection {
	if deps.Repos.Connection == nil {
		return nil
	}
	conns, err := deps.Repos.Connection.List(ctx, tenantID, connType)
	if err != nil || len(conns) == 0 {
		return nil
	}
	for i := range conns {
		if conns[i].Status == "connected" {
			decrypted, err := deps.Repos.Connection.GetDecrypted(ctx, conns[i].ID, tenantID)
			if err == nil {
				return decrypted
			}
			return &conns[i]
		}
	}
	return &conns[0]
}
