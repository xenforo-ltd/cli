// Package auth handles authentication and OAuth flows.
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

	"github.com/xenforo-ltd/cli/internal/config"
)

// OAuthClient handles the OAuth authentication flow.
type OAuthClient struct {
	config     *config.OAuthConfig
	httpClient *http.Client
}

// NewOAuthClient creates a new OAuth client.
func NewOAuthClient(cfg *config.OAuthConfig) *OAuthClient {
	return &OAuthClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// AuthorizationURL generates the OAuth authorization URL.
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

// ExchangeCode exchanges an authorization code for a token.
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
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (status %d): %s: %w", resp.StatusCode, string(body), ErrAuthFailed)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return c.tokenFromResponse(&tokenResp), nil
}

// RefreshToken refreshes an expired access token.
func (c *OAuthClient) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", c.config.ClientID)
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Token, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed (status %d): %s: %w", resp.StatusCode, string(body), ErrAuthExpired)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
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

// IntrospectToken checks the validity and metadata of a token.
func (c *OAuthClient) IntrospectToken(ctx context.Context, accessToken string) (*IntrospectResponse, error) {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("token", accessToken)
	data.Set("client_id", c.config.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Introspect, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create introspect request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read introspect response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token introspection failed (status %d): %s: %w", resp.StatusCode, string(body), ErrAuthFailed)
	}

	var introspectResp IntrospectResponse
	if err := json.Unmarshal(body, &introspectResp); err != nil {
		return nil, fmt.Errorf("failed to parse introspect response: %w", err)
	}

	return &introspectResp, nil
}

// RevokeToken revokes an access or refresh token.
func (c *OAuthClient) RevokeToken(ctx context.Context, token string) error {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("token", token)
	data.Set("client_id", c.config.ClientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Revoke, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create revoke request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	defer resp.Body.Close()

	// Revocation should return 200 OK even if token was already invalid
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("token revocation failed (status %d; body unreadable): %w", resp.StatusCode, err)
		}

		return fmt.Errorf("token revocation failed (status %d): %s: %w", resp.StatusCode, body, ErrAuthFailed)
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
		BaseURL:      c.config.BaseURL,
	}
}

// CallbackResult holds the result of the OAuth callback.
type CallbackResult struct {
	Code  string
	State string
	Error string
}

// CallbackServer handles the OAuth redirect callback from the authorization server.
type CallbackServer struct {
	listener net.Listener
	server   *http.Server
	result   chan CallbackResult
	serveErr chan error
	path     string
}

// NewCallbackServer creates a new callback server.
func NewCallbackServer(ctx context.Context, path string) (*CallbackServer, error) {
	lc := &net.ListenConfig{}

	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
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

		ReadHeaderTimeout: 5 * time.Second,

		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return cs, nil
}

// RedirectURI returns the callback server's redirect URI.
func (cs *CallbackServer) RedirectURI() string {
	return fmt.Sprintf("http://%s%s", cs.listener.Addr().String(), cs.path)
}

// Start begins listening for the OAuth callback.
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

// WaitForCallback waits for the OAuth callback result.
func (cs *CallbackServer) WaitForCallback(ctx context.Context) (*CallbackResult, error) {
	select {
	case result := <-cs.result:
		return &result, nil
	case err := <-cs.serveErr:
		return nil, fmt.Errorf("callback server error: %w", err)
	case <-ctx.Done():
		return nil, fmt.Errorf("authentication timed out: %w", ctx.Err())
	}
}

// Shutdown gracefully shuts down the callback server.
func (cs *CallbackServer) Shutdown(ctx context.Context) error {
	if err := cs.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shut down callback server: %w", err)
	}

	return nil
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	result := CallbackResult{
		Code:  query.Get("code"),
		State: query.Get("state"),
		Error: query.Get("error"),
	}

	// Build redirect URL to the completion page on the XenForo site
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)

		return
	}

	baseURL := strings.TrimSuffix(cfg.OAuth.BaseURL, "/")
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
