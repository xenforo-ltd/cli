package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Environment != EnvProduction {
		t.Errorf("Environment = %q, want %q", cfg.Environment, EnvProduction)
	}
}

func TestValidateEnvironment(t *testing.T) {
	tests := []struct {
		env     string
		wantErr bool
	}{
		{"production", false},
		{"development", false},
		{"staging", true},
		{"", true},
		{"PRODUCTION", true}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			err := ValidateEnvironment(tt.env)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEnvironment(%q) error = %v, wantErr %v", tt.env, err, tt.wantErr)
			}
		})
	}
}

func TestGlobalFlags(t *testing.T) {
	Reset()
	current = Default()
	current = Default()

	SetFlags(GlobalFlags{
		NonInteractive: true,
		Verbose:        true,
	})

	flags := GetFlags()

	if !flags.NonInteractive {
		t.Error("NonInteractive = false, want true")
	}
	if !flags.Verbose {
		t.Error("Verbose = false, want true")
	}

	Reset()
	current = Default()
	current = Default()
}

func TestGetEffectiveEnvironment(t *testing.T) {
	Reset()
	current = Default()
	current = Default()

	env := GetEffectiveEnvironment()
	if env != EnvProduction {
		t.Errorf("GetEffectiveEnvironment() = %q, want %q", env, EnvProduction)
	}

	Reset()
	current = Default()
	current = Default()
}

func TestGetEffectiveBaseURL(t *testing.T) {
	Reset()
	current = Default()

	url := GetEffectiveBaseURL()
	if url != DefaultProductionURL {
		t.Errorf("GetEffectiveBaseURL() = %q, want %q", url, DefaultProductionURL)
	}

	Reset()
	current = Default()
	current = &Config{
		Environment: EnvDevelopment,
		Development: EnvironmentConfig{
			OAuth: OAuthSettings{
				BaseURL: "https://test.example.com/",
			},
		},
	}

	url = GetEffectiveBaseURL()
	if url != "https://test.example.com/" {
		t.Errorf("GetEffectiveBaseURL() = %q, want %q", url, "https://test.example.com/")
	}

	Reset()
	current = Default()
}

func TestIsNonInteractive(t *testing.T) {
	Reset()
	current = Default()

	if IsNonInteractive() {
		t.Error("IsNonInteractive() = true, want false")
	}

	SetFlags(GlobalFlags{NonInteractive: true})
	if !IsNonInteractive() {
		t.Error("IsNonInteractive() = false, want true")
	}

	Reset()
	current = Default()
}

func TestIsVerbose(t *testing.T) {
	Reset()
	current = Default()

	if IsVerbose() {
		t.Error("IsVerbose() = true, want false")
	}

	SetFlags(GlobalFlags{Verbose: true})
	if !IsVerbose() {
		t.Error("IsVerbose() = false, want true")
	}

	Reset()
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "xf-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	Reset()
	current = Default()

	cfg := &Config{
		Environment: EnvDevelopment,
		CachePath:   "/custom/cache",
		Development: EnvironmentConfig{
			OAuth: OAuthSettings{
				BaseURL:  "https://test.example.com/",
				ClientID: "test-client",
			},
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	configPath := filepath.Join(tmpDir, ".config", "xf", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	Reset()

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Environment != EnvDevelopment {
		t.Errorf("Environment = %q, want %q", loaded.Environment, EnvDevelopment)
	}
	if loaded.Development.OAuth.BaseURL != "https://test.example.com/" {
		t.Errorf("Development.OAuth.BaseURL = %q, want %q", loaded.Development.OAuth.BaseURL, "https://test.example.com/")
	}
	if loaded.CachePath != "/custom/cache" {
		t.Errorf("CachePath = %q, want %q", loaded.CachePath, "/custom/cache")
	}

	Reset()
	current = Default()
}

func TestLoadMissingConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "xf-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	Reset()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Environment != EnvProduction {
		t.Errorf("Environment = %q, want %q (default)", cfg.Environment, EnvProduction)
	}

	Reset()
	current = Default()
}

func TestGetEffectiveClientID(t *testing.T) {
	Reset()
	current = Default()

	clientID := GetEffectiveClientID()
	if clientID != DefaultProductionClientID {
		t.Errorf("GetEffectiveClientID() = %q, want %q", clientID, DefaultProductionClientID)
	}

	Reset()
	current = Default()
	current = &Config{
		Environment: EnvDevelopment,
		Development: EnvironmentConfig{
			OAuth: OAuthSettings{
				ClientID: "test-client",
			},
		},
	}

	clientID = GetEffectiveClientID()
	if clientID != "test-client" {
		t.Errorf("GetEffectiveClientID() = %q, want %q", clientID, "test-client")
	}

	Reset()
	current = Default()
}

func TestGetEffectiveScopes(t *testing.T) {
	Reset()
	current = Default()

	scopes := GetEffectiveScopes()
	if len(scopes) != 1 || scopes[0] != "licenses:read" {
		t.Errorf("GetEffectiveScopes() = %v, want [licenses:read]", scopes)
	}

	Reset()
	current = Default()
}

func TestGetEffectiveRedirectPath(t *testing.T) {
	Reset()
	current = Default()

	path := GetEffectiveRedirectPath()
	if path != "/customer-oauth/complete" {
		t.Errorf("GetEffectiveRedirectPath() = %q, want %q", path, "/customer-oauth/complete")
	}

	Reset()
	current = Default()
}
