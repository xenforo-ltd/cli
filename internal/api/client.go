package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"xf/internal/auth"
	"xf/internal/config"
	"xf/internal/errors"
	"xf/internal/version"
)

// Client is an authenticated HTTP client for the XenForo Customer API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	keychain   tokenStore
	oauthCfg   *auth.OAuthConfig
	refreshFn  func(ctx context.Context, staleToken string) error

	mu sync.Mutex
}

type tokenStore interface {
	LoadToken() (*auth.Token, error)
	SaveToken(token *auth.Token) error
}

func NewClient() (*Client, error) {
	token, err := auth.RequireAuth()
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL: strings.TrimSuffix(token.BaseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		keychain: auth.NewKeychain(),
		oauthCfg: &auth.OAuthConfig{
			BaseURL:  token.BaseURL,
			ClientID: config.GetEffectiveClientID(),
		},
	}, nil
}

func userAgent() string {
	v := version.Get()
	return fmt.Sprintf("xf/%s (%s/%s)", v.Version, v.OS, v.Arch)
}

func (c *Client) Do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	var bodyBytes []byte
	if body != nil {
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, errors.Wrap(errors.CodeAPIRequestFailed, "failed to read request body", err)
		}
		bodyBytes = data
	}
	return c.doWithRetry(ctx, method, path, bodyBytes, true)
}

func (c *Client) doWithRetry(ctx context.Context, method, path string, body []byte, allowRetry bool) (*http.Response, error) {
	token, err := c.keychain.LoadToken()
	if err != nil {
		return nil, err
	}

	url := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, errors.Wrap(errors.CodeAPIRequestFailed, "failed to create request", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))
	req.Header.Set("User-Agent", userAgent())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(errors.CodeAPIRequestFailed, "request failed", err)
	}

	if resp.StatusCode == http.StatusUnauthorized && allowRetry {
		resp.Body.Close()

		refresh := c.refreshFn
		if refresh == nil {
			refresh = c.refreshToken
		}
		if err := refresh(ctx, token.AccessToken); err != nil {
			return nil, errors.Wrap(errors.CodeAuthExpired,
				"authentication expired and refresh failed - run 'xf auth login'", err)
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
		return err
	}

	if token.AccessToken != staleToken {
		return nil
	}

	if token.RefreshToken == "" {
		return errors.New(errors.CodeAuthExpired, "no refresh token available")
	}

	oauthClient := auth.NewOAuthClient(c.oauthCfg)
	newToken, err := oauthClient.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return err
	}

	return c.keychain.SaveToken(newToken)
}

func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.Do(ctx, http.MethodGet, path, nil)
}

func (c *Client) Post(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	return c.Do(ctx, http.MethodPost, path, body)
}

func (c *Client) PostJSON(ctx context.Context, path string, body []byte) (*http.Response, error) {
	return c.doWithRetry(ctx, http.MethodPost, path, body, true)
}

// APIError represents an error response from the API.
type APIError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// APIErrorResponse represents the error response structure from XenForo API.
type APIErrorResponse struct {
	Errors []APIError `json:"errors"`
}

func ParseError(body []byte) (*APIErrorResponse, error) {
	var errResp APIErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return nil, err
	}
	return &errResp, nil
}

func CheckResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Newf(errors.CodeAPIResponseInvalid, "API error (status %d): failed to read response", resp.StatusCode)
	}

	errResp, parseErr := ParseError(body)
	if parseErr != nil || len(errResp.Errors) == 0 {
		return errors.Newf(errors.CodeAPIRequestFailed, "API error (status %d): %s", resp.StatusCode, string(body))
	}

	return errors.Newf(errors.CodeAPIRequestFailed, "API error: %s", errResp.Errors[0].Message)
}
