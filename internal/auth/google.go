package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pepa/pepa/internal/config"
)

// Google well-known OIDC issuer.
const googleIssuer = "https://accounts.google.com"

// GoogleUserInfo represents user information from Google.
type GoogleUserInfo struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

// EffectiveEmail returns the user's email.
func (u *GoogleUserInfo) EffectiveEmail() string {
	return u.Email
}

// EffectiveName returns the best available display name.
func (u *GoogleUserInfo) EffectiveName() string {
	if u.Name != "" {
		return u.Name
	}
	if u.GivenName != "" {
		if u.FamilyName != "" {
			return u.GivenName + " " + u.FamilyName
		}
		return u.GivenName
	}
	return u.Email
}

// ExternalID returns the stable unique identifier (Google "sub" claim).
func (u *GoogleUserInfo) ExternalID() string {
	return u.Sub
}

// GoogleProvider wraps the standard OIDCProvider with Google-specific config.
type GoogleProvider struct {
	*OIDCProvider
}

// NewGoogleProvider creates a Google OAuth provider.
// It reuses the generic OIDC flow with Google's well-known issuer.
func NewGoogleProvider(cfg config.GoogleConfig) *GoogleProvider {
	oidcCfg := config.OIDCConfig{
		Enabled:      cfg.Enabled,
		Issuer:       googleIssuer,
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{"openid", "profile", "email"},
	}
	return &GoogleProvider{
		OIDCProvider: NewOIDCProvider(oidcCfg),
	}
}

// GetGoogleUserInfo fetches user info from Google's userinfo endpoint.
func (p *GoogleProvider) GetGoogleUserInfo(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	discovery, err := p.Discover(ctx)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discovery.UserinfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch userinfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}

	return &userInfo, nil
}
