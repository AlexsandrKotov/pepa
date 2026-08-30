package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pepa/pepa/internal/config"
)

// Azure well-known endpoint templates.
const (
	azureSingleTenantIssuer = "https://login.microsoftonline.com/%s/v2.0"
	azureMultiTenantIssuer  = "https://login.microsoftonline.com/common/v2.0"
)

// AzureOIDCConfig returns an OIDCConfig derived from the Azure AD settings.
// This allows reusing the existing OIDCProvider for Azure AD authentication.
func AzureOIDCConfig(cfg config.AzureADConfig) config.OIDCConfig {
	issuer := azureIssuer(cfg.TenantID)
	return config.OIDCConfig{
		Enabled:      cfg.Enabled,
		Issuer:       issuer,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{"openid", "profile", "email"},
	}
}

// azureIssuer builds the Microsoft identity platform issuer URL.
func azureIssuer(tenantID string) string {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" || tenantID == "common" {
		return azureMultiTenantIssuer
	}
	return fmt.Sprintf(azureSingleTenantIssuer, tenantID)
}

// AzureUserInfo extracts user info from OIDC claims with Azure-specific
// claim mapping. Azure AD uses "oid" for the unique user identifier and
// "upn" or "preferred_username" for the email equivalent.
type AzureUserInfo struct {
	Sub               string `json:"sub"`
	OID               string `json:"oid"` // Azure object ID (stable unique identifier)
	Email             string `json:"email"`
	UPN               string `json:"upn"` // User principal name (e.g. user@domain.com)
	PreferredUsername  string `json:"preferred_username"`
	Name              string `json:"name"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	TenantID          string `json:"tid"`
}

// EffectiveEmail returns the best available email from Azure claims.
func (u *AzureUserInfo) EffectiveEmail() string {
	if u.Email != "" {
		return u.Email
	}
	if u.UPN != "" {
		return u.UPN
	}
	return u.PreferredUsername
}

// EffectiveName returns the best available display name.
func (u *AzureUserInfo) EffectiveName() string {
	if u.Name != "" {
		return u.Name
	}
	if u.GivenName != "" {
		if u.FamilyName != "" {
			return u.GivenName + " " + u.FamilyName
		}
		return u.GivenName
	}
	if u.PreferredUsername != "" {
		return u.PreferredUsername
	}
	return u.EffectiveEmail()
}

// ExternalID returns the stable unique identifier for the user.
// Azure AD's "oid" claim is the immutable object ID.
func (u *AzureUserInfo) ExternalID() string {
	if u.OID != "" {
		return u.OID
	}
	return u.Sub
}

// AzureProvider wraps the standard OIDCProvider with Azure-specific config.
type AzureProvider struct {
	*OIDCProvider
	azureConfig config.AzureADConfig
}

// NewAzureProvider creates an Azure AD authentication provider.
func NewAzureProvider(cfg config.AzureADConfig) *AzureProvider {
	oidcCfg := AzureOIDCConfig(cfg)
	return &AzureProvider{
		OIDCProvider: NewOIDCProvider(oidcCfg),
		azureConfig:  cfg,
	}
}

// GetAzureUserInfo fetches user info and maps Azure-specific claims.
func (p *AzureProvider) GetAzureUserInfo(ctx context.Context, accessToken string) (*AzureUserInfo, error) {
	discovery, err := p.Discover(ctx)
	if err != nil {
		return nil, err
	}

	return getAzureUserInfoFromEndpoint(ctx, p.httpClient, discovery.UserinfoEndpoint, accessToken)
}

// getAzureUserInfoFromEndpoint fetches and decodes Azure user info from the userinfo endpoint.
func getAzureUserInfoFromEndpoint(ctx context.Context, client *http.Client, userinfoEndpoint, accessToken string) (*AzureUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo AzureUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}

	return &userInfo, nil
}
