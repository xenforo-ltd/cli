package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
)

func resetGlobals(t *testing.T) {
	t.Helper()

	viper.Reset()

	cacheOnce = sync.Once{}
	cache = Config{}
	errCache = nil
}

func TestOAuthEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		wantAuth   string
		wantToken  string
		wantIntro  string
		wantRevoke string
	}{
		{
			name:       "without trailing slash",
			baseURL:    "https://example.com",
			wantAuth:   "https://example.com/customer-oauth/authorize",
			wantToken:  "https://example.com/api/customer-oauth2/token",
			wantIntro:  "https://example.com/api/customer-oauth2/introspect",
			wantRevoke: "https://example.com/api/customer-oauth2/revoke",
		},
		{
			name:       "with trailing slash",
			baseURL:    "https://example.com/",
			wantAuth:   "https://example.com/customer-oauth/authorize",
			wantToken:  "https://example.com/api/customer-oauth2/token",
			wantIntro:  "https://example.com/api/customer-oauth2/introspect",
			wantRevoke: "https://example.com/api/customer-oauth2/revoke",
		},
		{
			name:       "with subpath",
			baseURL:    "https://example.com/sub",
			wantAuth:   "https://example.com/sub/customer-oauth/authorize",
			wantToken:  "https://example.com/sub/api/customer-oauth2/token",
			wantIntro:  "https://example.com/sub/api/customer-oauth2/introspect",
			wantRevoke: "https://example.com/sub/api/customer-oauth2/revoke",
		},
		{
			name:       "empty base url",
			baseURL:    "",
			wantAuth:   "/customer-oauth/authorize",
			wantToken:  "/api/customer-oauth2/token",
			wantIntro:  "/api/customer-oauth2/introspect",
			wantRevoke: "/api/customer-oauth2/revoke",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &OAuthConfig{BaseURL: tt.baseURL}
			ep := cfg.Endpoints()

			if ep.Auth != tt.wantAuth {
				t.Errorf("Auth = %q, want %q", ep.Auth, tt.wantAuth)
			}

			if ep.Token != tt.wantToken {
				t.Errorf("Token = %q, want %q", ep.Token, tt.wantToken)
			}

			if ep.Introspect != tt.wantIntro {
				t.Errorf("Introspect = %q, want %q", ep.Introspect, tt.wantIntro)
			}

			if ep.Revoke != tt.wantRevoke {
				t.Errorf("Revoke = %q, want %q", ep.Revoke, tt.wantRevoke)
			}
		})
	}
}

func TestInit_WithConfigFile(t *testing.T) {
	resetGlobals(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgFile, []byte(`{"verbose": true, "cache_path": "/tmp/test-cache"}`), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := Init(cfgFile); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if got := viper.GetBool("verbose"); !got {
		t.Error("expected verbose to be true from config file")
	}

	if got := viper.GetString("cache_path"); got != "/tmp/test-cache" {
		t.Errorf("cache_path = %q, want %q", got, "/tmp/test-cache")
	}
}

func TestInit_Defaults(t *testing.T) {
	resetGlobals(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgFile, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := Init(cfgFile); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if got := viper.GetBool("verbose"); got {
		t.Error("expected verbose default to be false")
	}

	if got := viper.GetBool("no_interaction"); got {
		t.Error("expected no_interaction default to be false")
	}

	if got := viper.GetString("oauth.base_url"); got != "https://xenforo.com/" {
		t.Errorf("oauth.base_url = %q, want %q", got, "https://xenforo.com/")
	}

	if got := viper.GetString("oauth.client_id"); got != "5062897895166491" {
		t.Errorf("oauth.client_id = %q, want %q", got, "5062897895166491")
	}

	if got := viper.GetString("oauth.redirect_path"); got != "/customer-oauth/complete" {
		t.Errorf("oauth.redirect_path = %q, want %q", got, "/customer-oauth/complete")
	}
}

func TestInit_MissingConfigFile(t *testing.T) {
	resetGlobals(t)

	err := Init("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("Init() expected error for missing config file, got nil")
	}
}

func TestLoad_UnmarshalsConfig(t *testing.T) {
	resetGlobals(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	content := `{
		"verbose": true,
		"no_interaction": true,
		"cache_path": "/custom/cache",
		"oauth": {
			"base_url": "https://custom.example.com/",
			"client_id": "test-client-id",
			"scopes": ["scope1", "scope2"],
			"redirect_path": "/callback"
		}
	}`

	if err := os.WriteFile(cfgFile, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := Init(cfgFile); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.Verbose {
		t.Error("expected Verbose to be true")
	}

	if !cfg.NoInteraction {
		t.Error("expected NoInteraction to be true")
	}

	if cfg.CachePath != "/custom/cache" {
		t.Errorf("CachePath = %q, want %q", cfg.CachePath, "/custom/cache")
	}

	if cfg.OAuth.BaseURL != "https://custom.example.com/" {
		t.Errorf("OAuth.BaseURL = %q, want %q", cfg.OAuth.BaseURL, "https://custom.example.com/")
	}

	if cfg.OAuth.ClientID != "test-client-id" {
		t.Errorf("OAuth.ClientID = %q, want %q", cfg.OAuth.ClientID, "test-client-id")
	}

	if len(cfg.OAuth.Scopes) != 2 || cfg.OAuth.Scopes[0] != "scope1" || cfg.OAuth.Scopes[1] != "scope2" {
		t.Errorf("OAuth.Scopes = %v, want [scope1 scope2]", cfg.OAuth.Scopes)
	}

	if cfg.OAuth.RedirectPath != "/callback" {
		t.Errorf("OAuth.RedirectPath = %q, want %q", cfg.OAuth.RedirectPath, "/callback")
	}
}

func TestLoad_UsesDefaults(t *testing.T) {
	resetGlobals(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgFile, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := Init(cfgFile); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Verbose {
		t.Error("expected Verbose default to be false")
	}

	if cfg.NoInteraction {
		t.Error("expected NoInteraction default to be false")
	}

	if cfg.OAuth.BaseURL != "https://xenforo.com/" {
		t.Errorf("OAuth.BaseURL = %q, want %q", cfg.OAuth.BaseURL, "https://xenforo.com/")
	}

	if cfg.OAuth.ClientID != "5062897895166491" {
		t.Errorf("OAuth.ClientID = %q, want %q", cfg.OAuth.ClientID, "5062897895166491")
	}

	if len(cfg.OAuth.Scopes) != 1 || cfg.OAuth.Scopes[0] != "licenses:read" {
		t.Errorf("OAuth.Scopes = %v, want [licenses:read]", cfg.OAuth.Scopes)
	}
}

func TestLoad_CachesResult(t *testing.T) {
	resetGlobals(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgFile, []byte(`{"verbose": true}`), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := Init(cfgFile); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cfg1, err := Load()
	if err != nil {
		t.Fatalf("Load() first call error = %v", err)
	}

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() second call error = %v", err)
	}

	if cfg1.Verbose != cfg2.Verbose || cfg1.CachePath != cfg2.CachePath || cfg1.OAuth.BaseURL != cfg2.OAuth.BaseURL {
		t.Error("expected Load() to return cached result on second call")
	}
}

func TestSave_WritesConfigFile(t *testing.T) {
	resetGlobals(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgFile, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := Init(cfgFile); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	viper.Set("verbose", true)

	if err := Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("expected saved config file to have content")
	}
}

func TestInit_EnvOverride(t *testing.T) {
	resetGlobals(t)

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgFile, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("XF_VERBOSE", "true")

	if err := Init(cfgFile); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if got := viper.GetBool("verbose"); !got {
		t.Error("expected verbose to be true from env var XF_VERBOSE")
	}
}
