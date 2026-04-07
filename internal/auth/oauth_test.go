package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/config"
)

func TestOAuthConfig_Endpoints(t *testing.T) {
	tests := []struct {
		name           string
		baseURL        string
		wantAuth       string
		wantToken      string
		wantIntrospect string
		wantRevoke     string
	}{
		{
			name:           "with trailing slash",
			baseURL:        "https://xenforo.com/",
			wantAuth:       "https://xenforo.com/customer-oauth/authorize",
			wantToken:      "https://xenforo.com/api/customer-oauth2/token",
			wantIntrospect: "https://xenforo.com/api/customer-oauth2/introspect",
			wantRevoke:     "https://xenforo.com/api/customer-oauth2/revoke",
		},
		{
			name:           "without trailing slash",
			baseURL:        "https://xenforo.com",
			wantAuth:       "https://xenforo.com/customer-oauth/authorize",
			wantToken:      "https://xenforo.com/api/customer-oauth2/token",
			wantIntrospect: "https://xenforo.com/api/customer-oauth2/introspect",
			wantRevoke:     "https://xenforo.com/api/customer-oauth2/revoke",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.OAuthConfig{BaseURL: tt.baseURL}
			endpoints := cfg.Endpoints()

			if endpoints.Auth != tt.wantAuth {
				t.Errorf("auth = %q, want %q", endpoints.Auth, tt.wantAuth)
			}

			if endpoints.Token != tt.wantToken {
				t.Errorf("token = %q, want %q", endpoints.Token, tt.wantToken)
			}

			if endpoints.Introspect != tt.wantIntrospect {
				t.Errorf("introspect = %q, want %q", endpoints.Introspect, tt.wantIntrospect)
			}

			if endpoints.Revoke != tt.wantRevoke {
				t.Errorf("revoke = %q, want %q", endpoints.Revoke, tt.wantRevoke)
			}
		})
	}
}

func TestOAuthClient_AuthorizationURL(t *testing.T) {
	cfg := &config.OAuthConfig{
		BaseURL:      "https://example.com/",
		ClientID:     "test-client",
		Scopes:       []string{"read", "write"},
		RedirectPath: "/callback",
	}
	client := NewOAuthClient(cfg)

	pkce := &PKCEParams{
		CodeVerifier:        "verifier123",
		CodeChallenge:       "challenge456",
		CodeChallengeMethod: "S256",
		State:               "state789",
	}

	uri := client.AuthorizationURL(pkce, "http://localhost:8080/callback")

	expectedParams := []string{
		"client_id=test-client",
		"response_type=code",
		"redirect_uri=" + url.QueryEscape("http://localhost:8080/callback"),
		"scope=read+write",
		"state=state789",
		"code_challenge=challenge456",
		"code_challenge_method=S256",
	}

	for _, param := range expectedParams {
		if !strings.Contains(uri, param) {
			t.Errorf("AuthorizationURL() missing param %q, got: %s", param, uri)
		}
	}
}

func TestOAuthClient_ExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		if r.URL.Path != "/api/customer-oauth2/token" {
			t.Errorf("Expected /api/customer-oauth2/token, got %s", r.URL.Path)
		}

		resp := TokenResponse{
			AccessToken:  "access123",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			RefreshToken: "refresh456",
			Scope:        "read write",
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	cfg := &config.OAuthConfig{
		BaseURL:  server.URL + "/",
		ClientID: "test-client",
	}
	client := NewOAuthClient(cfg)

	pkce := &PKCEParams{
		CodeVerifier: "verifier123",
	}

	ctx := t.Context()

	token, err := client.ExchangeCode(ctx, "auth-code", pkce, "http://localhost/callback")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}

	if token.AccessToken != "access123" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "access123")
	}

	if token.RefreshToken != "refresh456" {
		t.Errorf("RefreshToken = %q, want %q", token.RefreshToken, "refresh456")
	}

	if token.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", token.TokenType, "Bearer")
	}
}

func TestOAuthClient_RefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := TokenResponse{
			AccessToken:  "new-access-token",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			RefreshToken: "new-refresh-token",
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	cfg := &config.OAuthConfig{
		BaseURL:  server.URL + "/",
		ClientID: "test-client",
	}
	client := NewOAuthClient(cfg)

	ctx := t.Context()

	token, err := client.RefreshToken(ctx, "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}

	if token.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "new-access-token")
	}
}

func TestOAuthClient_IntrospectToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := IntrospectResponse{
			Active:   true,
			Username: "testuser",
			Scope:    "read write",
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	cfg := &config.OAuthConfig{
		BaseURL:  server.URL + "/",
		ClientID: "test-client",
	}
	client := NewOAuthClient(cfg)

	ctx := t.Context()

	resp, err := client.IntrospectToken(ctx, "some-token")
	if err != nil {
		t.Fatalf("IntrospectToken() error = %v", err)
	}

	if !resp.Active {
		t.Error("Active = false, want true")
	}

	if resp.Username != "testuser" {
		t.Errorf("Username = %q, want %q", resp.Username, "testuser")
	}
}

func TestOAuthClient_RevokeToken(t *testing.T) {
	called := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.OAuthConfig{
		BaseURL:  server.URL + "/",
		ClientID: "test-client",
	}
	client := NewOAuthClient(cfg)

	ctx := t.Context()

	err := client.RevokeToken(ctx, "some-token")
	if err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}

	if !called {
		t.Error("Server was not called")
	}
}

func TestCallbackServer(t *testing.T) {
	server, err := NewCallbackServer(t.Context(), "/callback")
	if err != nil {
		t.Fatalf("NewCallbackServer() error = %v", err)
	}

	uri := server.RedirectURI()
	if !strings.HasPrefix(uri, "http://127.0.0.1:") {
		t.Errorf("RedirectURI() = %q, want http://127.0.0.1:...", uri)
	}

	if !strings.HasSuffix(uri, "/callback") {
		t.Errorf("RedirectURI() = %q, want .../callback", uri)
	}

	server.Start()

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	sendCallback := func(query string) <-chan error {
		done := make(chan error, 1)

		go func() {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, uri+query, nil)
			if err != nil {
				done <- err
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				done <- err
				return
			}
			defer resp.Body.Close()

			_, _ = io.Copy(io.Discard, resp.Body)

			done <- nil
		}()

		return done
	}

	defer func() {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			t.Logf("callback server shutdown: %v", err)
		}
	}()

	firstCallbackDone := sendCallback("?code=test-code&state=test-state")

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	result, err := server.WaitForCallback(ctx)
	if err != nil {
		t.Fatalf("WaitForCallback() error = %v", err)
	}

	if result.Code != "test-code" {
		t.Errorf("Code = %q, want %q", result.Code, "test-code")
	}

	if result.State != "test-state" {
		t.Errorf("State = %q, want %q", result.State, "test-state")
	}

	if err := <-firstCallbackDone; err != nil {
		t.Fatalf("Failed to make first HTTP request: %v", err)
	}

	secondCallbackDone := sendCallback("?code=second-code&state=test-state")

	if err := <-secondCallbackDone; err != nil {
		t.Fatalf("Failed to make second HTTP request: %v", err)
	}
}

func TestCallbackServer_Timeout(t *testing.T) {
	server, err := NewCallbackServer(t.Context(), "/callback")
	if err != nil {
		t.Fatalf("NewCallbackServer() error = %v", err)
	}

	server.Start()

	defer func() {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	}()

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	_, err = server.WaitForCallback(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForCallback() error = %v, want DeadlineExceeded", err)
	}
}
