// Package customerapi provides an authenticated HTTP client for the XenForo Customer API.
package customerapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/version"
)

var (
	// ErrAPIError indicates the API returned an error response.
	ErrAPIError = errors.New("API error")

	// ErrAuthExpired indicates the authentication token has expired.
	ErrAuthExpired = errors.New("authentication expired")
)

// Client is an authenticated HTTP client for the XenForo Customer API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	keychain   tokenStore
	oauthCfg   *config.OAuthConfig
	refreshFn  func(ctx context.Context, staleToken string) error

	mu sync.Mutex
}

type tokenStore interface {
	LoadToken() (*auth.Token, error)
	SaveToken(token *auth.Token) error
}

// NewClient creates a new API client with authentication.
func NewClient() (*Client, error) {
	token, err := auth.RequireAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to load authentication token: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load customer API configuration: %w", err)
	}

	return &Client{
		baseURL: strings.TrimSuffix(token.BaseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		keychain: auth.NewKeychain(),
		oauthCfg: &config.OAuthConfig{
			BaseURL:  token.BaseURL,
			ClientID: cfg.OAuth.ClientID,
		},
	}, nil
}

func userAgent() string {
	v := version.Get()
	return fmt.Sprintf("xenforo-cli/%s (%s/%s)", v.Version, v.OS, v.Arch)
}

// Do sends an HTTP request and returns the response.
func (c *Client) Do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	var bodyBytes []byte

	if body != nil {
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}

		bodyBytes = data
	}

	return c.doWithRetry(ctx, method, path, bodyBytes, true)
}

// Get sends a GET request.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.Do(ctx, http.MethodGet, path, nil)
}

// Post sends a POST request.
func (c *Client) Post(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	return c.Do(ctx, http.MethodPost, path, body)
}

// PostJSON sends a POST request with JSON body.
func (c *Client) PostJSON(ctx context.Context, path string, body []byte) (*http.Response, error) {
	return c.doWithRetry(ctx, http.MethodPost, path, body, true)
}

// GetJSON performs a GET request and decodes the JSON response into result.
func (c *Client) GetJSON(ctx context.Context, path string, result any) error {
	resp, err := c.Get(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := CheckResponse(resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode API response for %s: %w", path, err)
	}

	return nil
}

func (c *Client) doWithRetry(ctx context.Context, method, path string, body []byte, allowRetry bool) (*http.Response, error) {
	token, err := c.keychain.LoadToken()
	if err != nil {
		return nil, fmt.Errorf("failed to load authentication token from keychain: %w", err)
	}

	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Accept", "application/json")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized && allowRetry {
		resp.Body.Close()

		refresh := c.refreshFn
		if refresh == nil {
			refresh = c.refreshToken
		}

		if err := refresh(ctx, token.AccessToken); err != nil {
			return nil, fmt.Errorf("authentication expired and refresh failed - run 'xf auth login': %w", err)
		}

		return c.doWithRetry(ctx, method, path, body, false)
	}

	return resp, nil
}

func (c *Client) refreshToken(ctx context.Context, staleToken string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	token, err := c.keychain.LoadToken()
	if err != nil {
		return fmt.Errorf("failed to load authentication token from keychain: %w", err)
	}

	if token.AccessToken != staleToken {
		return nil
	}

	if token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available: %w", ErrAuthExpired)
	}

	oauthClient := auth.NewOAuthClient(c.oauthCfg)

	newToken, err := oauthClient.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return fmt.Errorf("failed to refresh authentication token: %w", err)
	}

	if err := c.keychain.SaveToken(newToken); err != nil {
		return fmt.Errorf("failed to save refreshed authentication token: %w", err)
	}

	return nil
}

// Error represents an error response from the API.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Params  map[string]any `json:"params,omitempty"`
}

// ErrorResponse represents the error response structure from XenForo API.
type ErrorResponse struct {
	Errors []Error `json:"errors"`
}

// ParseError parses an API error response.
func ParseError(body []byte) (*ErrorResponse, error) {
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return nil, fmt.Errorf("failed to parse API error response: %w", err)
	}

	return &errResp, nil
}

// CheckResponse checks if an HTTP response indicates an error and returns a formatted error.
func CheckResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("API error (status %d): failed to read response: %w", resp.StatusCode, err)
	}

	errResp, parseErr := ParseError(body)
	if parseErr != nil || len(errResp.Errors) == 0 {
		return fmt.Errorf("API error (status %d): %s: %w", resp.StatusCode, string(body), parseErr)
	}

	return fmt.Errorf("API error: %s: %w", errResp.Errors[0].Message, ErrAPIError)
}
