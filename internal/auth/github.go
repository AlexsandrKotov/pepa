package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pepa/pepa/internal/config"
)

// GitHub OAuth2 endpoints (not OIDC — GitHub does not support OIDC discovery).
const (
	githubAuthURL     = "https://github.com/login/oauth/authorize"
	githubTokenURL    = "https://github.com/login/oauth/access_token" // #nosec //nolint:gosec // G101: this is a URL, not a credential
	githubUserAPI     = "https://api.github.com/user"
	githubEmailsAPI   = "https://api.github.com/user/emails"
)

// GitHubUserInfo represents user information from GitHub.
type GitHubUserInfo struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// EffectiveEmail returns the user's email.
func (u *GitHubUserInfo) EffectiveEmail() string {
	return u.Email
}

// EffectiveName returns the best available display name.
func (u *GitHubUserInfo) EffectiveName() string {
	if u.Name != "" {
		return u.Name
	}
	if u.Login != "" {
		return u.Login
	}
	return u.Email
}

// ExternalID returns the stable unique identifier (GitHub numeric user ID).
func (u *GitHubUserInfo) ExternalID() string {
	return fmt.Sprintf("%d", u.ID)
}

// GitHubProvider handles GitHub OAuth2 authentication.
type GitHubProvider struct {
	config     config.GitHubConfig
	httpClient *http.Client
}

// NewGitHubProvider creates a new GitHub OAuth provider.
func NewGitHubProvider(cfg config.GitHubConfig) *GitHubProvider {
	return &GitHubProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// BuildAuthURL constructs the GitHub authorization URL.
func (p *GitHubProvider) BuildAuthURL(_ context.Context, state string) (string, error) {
	params := url.Values{
		"client_id":    {p.config.ClientID},
		"redirect_uri": {p.config.RedirectURL},
		"scope":        {"read:user user:email"},
		"state":        {state},
	}
	return githubAuthURL + "?" + params.Encode(), nil
}

// githubTokenResponse represents the token response from GitHub.
type githubTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// ExchangeCode exchanges the authorization code for an access token.
func (p *GitHubProvider) ExchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{
		"client_id":     {p.config.ClientID},
		"client_secret": {p.config.ClientSecret},
		"code":          {code},
		"redirect_uri":  {p.config.RedirectURL},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchange code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp githubTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tokenResp.Error != "" {
		return "", fmt.Errorf("token error: %s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token in response")
	}

	return tokenResp.AccessToken, nil
}

// GetGitHubUserInfo fetches user information from GitHub.
func (p *GitHubProvider) GetGitHubUserInfo(ctx context.Context, accessToken string) (*GitHubUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubUserAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("create user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch user info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo GitHubUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}

	// GitHub may not return email in the user endpoint if it's set to private.
	// Fetch from the emails endpoint if needed.
	if userInfo.Email == "" {
		email, fetchErr := p.fetchPrimaryEmail(ctx, accessToken)
		if fetchErr == nil && email != "" {
			userInfo.Email = email
		}
	}

	return &userInfo, nil
}

// githubEmail represents an email entry from GitHub's emails endpoint.
type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// fetchPrimaryEmail retrieves the user's primary verified email from GitHub.
func (p *GitHubProvider) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubEmailsAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("emails endpoint returned status %d", resp.StatusCode)
	}

	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	// Prefer primary+verified, fall back to just primary, then any verified
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	return "", nil
}
