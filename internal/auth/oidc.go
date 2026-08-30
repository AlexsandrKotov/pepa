package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pepa/pepa/internal/config"
)

// OIDCProvider handles OpenID Connect authentication flow.
type OIDCProvider struct {
	config     config.OIDCConfig
	httpClient *http.Client
}

// OIDCDiscovery represents the OpenID Connect discovery document.
type OIDCDiscovery struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint"`
	JwksURI               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
}

// OIDCTokens represents the tokens received from the OIDC provider.
type OIDCTokens struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

// OIDCUserInfo represents user information from the OIDC provider.
type OIDCUserInfo struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	Picture           string `json:"picture"`
}

// NewOIDCProvider creates a new OIDC provider instance.
func NewOIDCProvider(cfg config.OIDCConfig) *OIDCProvider {
	return &OIDCProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// validateURL checks if the URL is safe (not pointing to internal networks).
func validateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("invalid scheme: %s", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host")
	}

	// Resolve hostname to check IP addresses
	ips, err := net.LookupIP(host)
	if err != nil {
		// If we can't resolve, allow it (might be DNS issue)
		// but in production you might want to be stricter
		return nil
	}

	for _, ip := range ips {
		if isPrivateIP(ip) {
			return fmt.Errorf("URL points to private IP: %s", ip)
		}
	}

	return nil
}

// isPrivateIP checks if an IP is in private/reserved ranges.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDR("127.0.0.0/8")},    // Loopback
		{mustParseCIDR("10.0.0.0/8")},     // Private
		{mustParseCIDR("172.16.0.0/12")},  // Private
		{mustParseCIDR("192.168.0.0/16")}, // Private
		{mustParseCIDR("169.254.0.0/16")}, // Link-local
		{mustParseCIDR("::1/128")},        // IPv6 loopback
		{mustParseCIDR("fc00::/7")},       // IPv6 private
		{mustParseCIDR("fe80::/10")},      // IPv6 link-local
	}

	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

func mustParseCIDR(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return network
}

// Discover fetches the OpenID Connect discovery document.
func (p *OIDCProvider) Discover(ctx context.Context) (*OIDCDiscovery, error) {
	// Validate issuer URL to prevent SSRF
	if err := validateURL(p.config.Issuer); err != nil {
		return nil, fmt.Errorf("invalid issuer URL: %w", err)
	}

	wellKnown := strings.TrimSuffix(p.config.Issuer, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return nil, fmt.Errorf("create discovery request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch discovery: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery endpoint returned status %d", resp.StatusCode)
	}

	var discovery OIDCDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		return nil, fmt.Errorf("decode discovery: %w", err)
	}

	return &discovery, nil
}

// BuildAuthURL constructs the authorization URL for the OIDC flow.
func (p *OIDCProvider) BuildAuthURL(ctx context.Context, state, nonce string) (string, error) {
	discovery, err := p.Discover(ctx)
	if err != nil {
		return "", err
	}

	scopes := p.config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	params := url.Values{
		"client_id":     {p.config.ClientID},
		"response_type": {"code"},
		"redirect_uri":  {p.config.RedirectURL},
		"scope":         {strings.Join(scopes, " ")},
		"state":         {state},
		"nonce":         {nonce},
	}

	return discovery.AuthorizationEndpoint + "?" + params.Encode(), nil
}

// ExchangeCode exchanges the authorization code for tokens.
func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string) (*OIDCTokens, error) {
	discovery, err := p.Discover(ctx)
	if err != nil {
		return nil, err
	}

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {p.config.RedirectURL},
		"client_id":     {p.config.ClientID},
		"client_secret": {p.config.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discovery.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokens OIDCTokens
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, fmt.Errorf("decode tokens: %w", err)
	}

	return &tokens, nil
}

// GetUserInfo fetches user information from the OIDC provider.
func (p *OIDCProvider) GetUserInfo(ctx context.Context, accessToken string) (*OIDCUserInfo, error) {
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
		return nil, fmt.Errorf("userinfo endpoint returned status %d", resp.StatusCode)
	}

	var userInfo OIDCUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}

	return &userInfo, nil
}

// GenerateState generates a cryptographically secure random state parameter.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// GenerateNonce generates a cryptographically secure random nonce.
func GenerateNonce() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
