package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/config"
)

// OAuthConfig holds OAuth endpoint configuration.
type OAuthConfig struct {
	// BaseURL is the base URL for all OAuth endpoints.
	BaseURL string

	// ClientID is the OAuth client identifier.
	ClientID string

	// Scopes are the requested OAuth scopes.
	Scopes []string

	// RedirectPath is the path for the OAuth callback.
	RedirectPath string
}

// OAuthEndpoints holds the OAuth endpoint URLs.
type OAuthEndpoints struct {
	Auth       string
	Token      string
	Introspect string
	Revoke     string
}

// DefaultOAuthConfig returns the OAuth configuration based on current environment settings.
func DefaultOAuthConfig() *OAuthConfig {
	return &OAuthConfig{
		BaseURL:      config.GetEffectiveBaseURL(),
		ClientID:     config.GetEffectiveClientID(),
		Scopes:       config.GetEffectiveScopes(),
		RedirectPath: config.GetEffectiveRedirectPath(),
	}
}

// Endpoints returns the OAuth endpoint URLs.
func (c *OAuthConfig) Endpoints() *OAuthEndpoints {
	base := strings.TrimSuffix(c.BaseURL, "/")
	return &OAuthEndpoints{
		Auth:       base + "/customer-oauth/authorize",
		Token:      base + "/api/customer-oauth2/token",
		Introspect: base + "/api/customer-oauth2/introspect",
		Revoke:     base + "/api/customer-oauth2/revoke",
	}
}

// OAuthClient handles the OAuth authentication flow.
type OAuthClient struct {
	config     *OAuthConfig
	httpClient *http.Client
}

// NewOAuthClient creates a new OAuth client.
func NewOAuthClient(cfg *OAuthConfig) *OAuthClient {
	if cfg == nil {
		cfg = DefaultOAuthConfig()
	}
	return &OAuthClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *OAuthClient) AuthorizationURL(pkce *PKCEParams, redirectURI string) string {
	endpoints := c.config.Endpoints()

	params := url.Values{}
	params.Set("client_id", c.config.ClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(c.config.Scopes, " "))
	params.Set("state", pkce.State)
	params.Set("code_challenge", pkce.CodeChallenge)
	params.Set("code_challenge_method", pkce.CodeChallengeMethod)

	return endpoints.Auth + "?" + params.Encode()
}

// TokenResponse represents the OAuth token response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

func (c *OAuthClient) ExchangeCode(ctx context.Context, code string, pkce *PKCEParams, redirectURI string) (*Token, error) {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", c.config.ClientID)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", pkce.CodeVerifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Token, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIRequestFailed, "failed to create token request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIRequestFailed, "failed to exchange code for token", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIResponseInvalid, "failed to read token response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, clierrors.Newf(clierrors.CodeAPIRequestFailed, "token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIResponseInvalid, "failed to parse token response", err)
	}

	return c.tokenFromResponse(&tokenResp), nil
}

func (c *OAuthClient) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", c.config.ClientID)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Token, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIRequestFailed, "failed to create refresh request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIRequestFailed, "failed to refresh token", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIResponseInvalid, "failed to read refresh response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, clierrors.Newf(clierrors.CodeAuthExpired, "token refresh failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIResponseInvalid, "failed to parse refresh response", err)
	}

	token := c.tokenFromResponse(&tokenResp)

	// If no new refresh token was provided, keep the old one
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}

	return token, nil
}

// IntrospectResponse represents the token introspection response.
type IntrospectResponse struct {
	Active    bool   `json:"active"`
	Scope     string `json:"scope,omitempty"`
	ClientID  string `json:"client_id,omitempty"`
	Username  string `json:"username,omitempty"`
	TokenType string `json:"token_type,omitempty"`
	Exp       int64  `json:"exp,omitempty"`
	Iat       int64  `json:"iat,omitempty"`
	Sub       string `json:"sub,omitempty"`
}

func (c *OAuthClient) IntrospectToken(ctx context.Context, accessToken string) (*IntrospectResponse, error) {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("token", accessToken)
	data.Set("client_id", c.config.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Introspect, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIRequestFailed, "failed to create introspect request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIRequestFailed, "failed to introspect token", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIResponseInvalid, "failed to read introspect response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, clierrors.Newf(clierrors.CodeAPIRequestFailed, "token introspection failed (status %d): %s", resp.StatusCode, string(body))
	}

	var introspectResp IntrospectResponse
	if err := json.Unmarshal(body, &introspectResp); err != nil {
		return nil, clierrors.Wrap(clierrors.CodeAPIResponseInvalid, "failed to parse introspect response", err)
	}

	return &introspectResp, nil
}

func (c *OAuthClient) RevokeToken(ctx context.Context, token string) error {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("token", token)
	data.Set("client_id", c.config.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Revoke, strings.NewReader(data.Encode()))
	if err != nil {
		return clierrors.Wrap(clierrors.CodeAPIRequestFailed, "failed to create revoke request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return clierrors.Wrap(clierrors.CodeAPIRequestFailed, "failed to revoke token", err)
	}
	defer resp.Body.Close()

	// Revocation should return 200 OK even if token was already invalid
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return clierrors.Newf(clierrors.CodeAPIRequestFailed, "token revocation failed (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *OAuthClient) tokenFromResponse(resp *TokenResponse) *Token {
	now := time.Now()
	return &Token{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresAt:    now.Add(time.Duration(resp.ExpiresIn) * time.Second),
		Scope:        resp.Scope,
		IssuedAt:     now,
		Environment:  string(config.GetEffectiveEnvironment()),
		BaseURL:      c.config.BaseURL,
	}
}

// CallbackResult holds the result of the OAuth callback.
type CallbackResult struct {
	Code  string
	State string
	Error string
}

// CallbackServer handles the OAuth redirect callback.
type CallbackServer struct {
	listener net.Listener
	server   *http.Server
	result   chan CallbackResult
	serveErr chan error
	path     string
}

// NewCallbackServer creates a new callback server.
func NewCallbackServer(path string) (*CallbackServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeInternal, "failed to start callback server", err)
	}

	cs := &CallbackServer{
		listener: listener,
		result:   make(chan CallbackResult, 1),
		serveErr: make(chan error, 1),
		path:     path,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, cs.handleCallback)

	cs.server = &http.Server{
		Handler: mux,
	}

	return cs, nil
}

func (cs *CallbackServer) RedirectURI() string {
	return fmt.Sprintf("http://%s%s", cs.listener.Addr().String(), cs.path)
}

func (cs *CallbackServer) Start() {
	go func() {
		err := cs.server.Serve(cs.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case cs.serveErr <- err:
			default:
			}
		}
	}()
}

func (cs *CallbackServer) WaitForCallback(ctx context.Context) (*CallbackResult, error) {
	select {
	case result := <-cs.result:
		return &result, nil
	case err := <-cs.serveErr:
		return nil, clierrors.Wrap(clierrors.CodeAuthFailed, "callback server error", err)
	case <-ctx.Done():
		return nil, clierrors.New(clierrors.CodeAuthFailed, "authentication timed out")
	}
}

func (cs *CallbackServer) Shutdown(ctx context.Context) error {
	return cs.server.Shutdown(ctx)
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	result := CallbackResult{
		Code:  query.Get("code"),
		State: query.Get("state"),
		Error: query.Get("error"),
	}

	// Build redirect URL to the completion page on the XenForo site
	baseURL := strings.TrimSuffix(config.GetEffectiveBaseURL(), "/")
	redirectURL := baseURL + "/customer-oauth/complete"

	if result.Error != "" {
		redirectURL += "?error=" + url.QueryEscape(result.Error)
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)

	// Send result to channel (CLI continues processing)
	select {
	case cs.result <- result:
	default:
	}
}
