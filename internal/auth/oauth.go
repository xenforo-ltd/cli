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

// oauthClient handles the OAuth authentication flow.
type oauthClient struct {
	config     *config.OAuthConfig
	httpClient *http.Client
}

// NewOAuthClient creates a new OAuth client.
func NewOAuthClient(cfg *config.OAuthConfig) *oauthClient {
	return &oauthClient{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// AuthorizationURL generates the OAuth authorization URL.
func (c *oauthClient) AuthorizationURL(pkce *pkceParams, redirectURI string) string {
	endpoints := c.config.Endpoints()

	params := url.Values{}
	params.Set("client_id", c.config.ClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(c.config.Scopes, " "))
	params.Set("state", pkce.State)
	params.Set("code_challenge", pkce.codeChallenge)
	params.Set("code_challenge_method", pkce.codeChallengeMethod)

	return endpoints.Auth + "?" + params.Encode()
}

// tokenResponse represents the OAuth token response.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// readTokenResponse decodes a token endpoint response. A non-200 status is
// reported as the operation failing, wrapped with sentinel.
func readTokenResponse(resp *http.Response, kind, operation string, sentinel error) (*tokenResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s response: %w", kind, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s failed (status %d): %s: %w", operation, resp.StatusCode, body, sentinel)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse %s response: %w", kind, err)
	}

	return &tokenResp, nil
}

// ExchangeCode exchanges an authorization code for a token.
func (c *oauthClient) ExchangeCode(ctx context.Context, code string, pkce *pkceParams, redirectURI string) (*Token, error) {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", c.config.ClientID)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", pkce.codeVerifier)

	resp, err := c.postForm(ctx, endpoints.Token, data, "token request", "exchange code for token")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	tokenResp, err := readTokenResponse(resp, "token", "token exchange", errAuthFailed)
	if err != nil {
		return nil, err
	}

	return c.tokenFromResponse(tokenResp), nil
}

// RefreshToken requests a new access token and retains the previous refresh
// token if the response does not provide a replacement.
func (c *oauthClient) RefreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("client_id", c.config.ClientID)
	data.Set("refresh_token", refreshToken)

	resp, err := c.postForm(ctx, endpoints.Token, data, "refresh request", "refresh token")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	tokenResp, err := readTokenResponse(resp, "refresh", "token refresh", errAuthExpired)
	if err != nil {
		return nil, err
	}

	token := c.tokenFromResponse(tokenResp)

	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}

	return token, nil
}

// IntrospectResponse represents the token introspection response.
type IntrospectResponse struct {
	Active   bool   `json:"active"`
	Username string `json:"username,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Exp      int64  `json:"exp,omitempty"`
	Iat      int64  `json:"iat,omitempty"`
}

// IntrospectToken checks the validity and metadata of a token.
func (c *oauthClient) IntrospectToken(ctx context.Context, accessToken string) (*IntrospectResponse, error) {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("token", accessToken)
	data.Set("client_id", c.config.ClientID)

	resp, err := c.postForm(ctx, endpoints.Introspect, data, "introspect request", "introspect token")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read introspect response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token introspection failed (status %d): %s: %w", resp.StatusCode, body, errAuthFailed)
	}

	var introspectResp IntrospectResponse
	if err := json.Unmarshal(body, &introspectResp); err != nil {
		return nil, fmt.Errorf("failed to parse introspect response: %w", err)
	}

	return &introspectResp, nil
}

// RevokeToken requests revocation of an access or refresh token and accepts
// only HTTP 200 as success.
func (c *oauthClient) RevokeToken(ctx context.Context, token string) error {
	endpoints := c.config.Endpoints()

	data := url.Values{}
	data.Set("token", token)
	data.Set("client_id", c.config.ClientID)

	resp, err := c.postForm(ctx, endpoints.Revoke, data, "revoke request", "revoke token")
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("token revocation failed (status %d; body unreadable): %w", resp.StatusCode, err)
		}

		return fmt.Errorf("token revocation failed (status %d): %s: %w", resp.StatusCode, body, errAuthFailed)
	}

	return nil
}

// postForm sends an OAuth form request; the caller must close the response body.
func (c *oauthClient) postForm(ctx context.Context, endpoint string, data url.Values, request, action string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", request, err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to %s: %w", action, err)
	}

	return resp, nil
}

// tokenFromResponse records the configured base URL and calculates expiry
// from the current time and the response's lifetime in seconds.
func (c *oauthClient) tokenFromResponse(resp *tokenResponse) *Token {
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

// callbackResult holds the result of the OAuth callback.
type callbackResult struct {
	Code  string
	State string
	Error string
}

// callbackServer handles the OAuth redirect callback from the authorization server.
type callbackServer struct {
	listener net.Listener
	server   *http.Server
	result   chan callbackResult
	serveErr chan error
	path     string
}

// NewCallbackServer creates a new callback server.
func NewCallbackServer(ctx context.Context, path string) (*callbackServer, error) {
	lc := &net.ListenConfig{}

	listener, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
	}

	cs := &callbackServer{
		listener: listener,
		result:   make(chan callbackResult, 1),
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
func (cs *callbackServer) RedirectURI() string {
	return fmt.Sprintf("http://%s%s", cs.listener.Addr(), cs.path)
}

// Start begins listening for the OAuth callback.
func (cs *callbackServer) Start() {
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
func (cs *callbackServer) WaitForCallback(ctx context.Context) (*callbackResult, error) {
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
func (cs *callbackServer) Shutdown(ctx context.Context) error {
	if err := cs.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shut down callback server: %w", err)
	}

	return nil
}

// handleCallback redirects the browser to the completion page and sends the
// callback result without blocking if the result channel is already full.
func (cs *callbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	result := callbackResult{
		Code:  query.Get("code"),
		State: query.Get("state"),
		Error: query.Get("error"),
	}

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

	select {
	case cs.result <- result:
	default:
	}
}
