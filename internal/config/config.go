// Package config manages CLI configuration and settings.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

// Environment represents the target environment.
type Environment string

const (
	// EnvProduction is the production environment.
	EnvProduction Environment = "production"

	// EnvDevelopment is the development environment.
	EnvDevelopment Environment = "development"
)

// Default OAuth client IDs for each environment.
const (
	DefaultProductionClientID  = "5062897895166491"
	DefaultDevelopmentClientID = ""
)

// Default base URLs for each environment.
const (
	DefaultProductionURL  = "https://xenforo.com/"
	DefaultDevelopmentURL = ""
)

// OAuthSettings holds OAuth configuration for an environment.
type OAuthSettings struct {
	// BaseURL is the base URL for OAuth endpoints.
	BaseURL string `json:"base_url,omitempty"`

	// ClientID is the OAuth client identifier.
	ClientID string `json:"client_id,omitempty"`

	// Scopes are the OAuth scopes to request.
	Scopes []string `json:"scopes,omitempty"`

	// RedirectPath is the path for the OAuth callback (default: /customer-oauth/complete).
	RedirectPath string `json:"redirect_path,omitempty"`
}

// EnvironmentConfig holds all settings for a specific environment.
type EnvironmentConfig struct {
	OAuth OAuthSettings `json:"oauth"`
}

// Config holds the CLI configuration.
type Config struct {
	// Environment is the default environment (production or development).
	Environment Environment `json:"environment"`

	// Production holds production environment settings.
	Production EnvironmentConfig `json:"production"`

	// Development holds development environment settings.
	Development EnvironmentConfig `json:"development"`

	// CachePath overrides the default cache directory.
	CachePath string `json:"cache_path,omitempty"`
}

// GlobalFlags holds command-line flags that affect configuration.
type GlobalFlags struct {
	NonInteractive bool
	Verbose        bool
}

var (
	current *Config
	mu      sync.RWMutex

	flags GlobalFlags
)

// DefaultConfigDir returns the default configuration directory path.
func DefaultConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", clierrors.Wrap(clierrors.CodeConfigReadFailed, "failed to get home directory", err)
	}
	return filepath.Join(homeDir, ".config", "xf"), nil
}

// DefaultCacheDir returns the default cache directory path.
func DefaultCacheDir() (string, error) {
	configDir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "cache"), nil
}

// FilePath returns the path to the configuration file.
func FilePath() (string, error) {
	configDir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "config.json"), nil
}

// Default returns a default configuration.
func Default() *Config {
	return &Config{
		Environment: EnvProduction,
	}
}

// Load loads the configuration from disk or returns default if not found.
func Load() (*Config, error) {
	mu.Lock()
	defer mu.Unlock()

	if current != nil {
		return current, nil
	}

	configPath, err := FilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			current = Default()
			return current, nil
		}
		return nil, clierrors.Wrap(clierrors.CodeConfigReadFailed, "failed to read config file", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, clierrors.Wrap(clierrors.CodeConfigInvalid, "failed to parse config file", err)
	}

	current = &cfg
	return current, nil
}

// Save persists the configuration to disk.
func Save(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()

	configPath, err := FilePath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return clierrors.Wrap(clierrors.CodeDirCreateFailed, "failed to create config directory", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return clierrors.Wrap(clierrors.CodeConfigWriteFailed, "failed to marshal config", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return clierrors.Wrap(clierrors.CodeConfigWriteFailed, "failed to write config file", err)
	}

	current = cfg
	return nil
}

// SetFlags sets global command-line flags.
func SetFlags(f GlobalFlags) {
	mu.Lock()
	defer mu.Unlock()
	flags = f
}

// GetFlags returns the current global command-line flags.
func GetFlags() GlobalFlags {
	mu.RLock()
	defer mu.RUnlock()
	return flags
}

// GetEffectiveEnvironment returns the effective environment from configuration.
func GetEffectiveEnvironment() Environment {
	cfg, err := Load()
	if err != nil {
		return EnvProduction
	}
	return cfg.Environment
}

// GetEnvironmentConfig returns configuration for a specific environment.
func GetEnvironmentConfig(env Environment) *EnvironmentConfig {
	cfg, err := Load()
	if err != nil {
		return &EnvironmentConfig{}
	}

	switch env {
	case EnvDevelopment:
		return &cfg.Development
	default:
		return &cfg.Production
	}
}

// GetEffectiveEnvironmentConfig returns the effective environment configuration.
func GetEffectiveEnvironmentConfig() *EnvironmentConfig {
	return GetEnvironmentConfig(GetEffectiveEnvironment())
}

// GetEffectiveBaseURL returns the configured base URL for the current environment.
func GetEffectiveBaseURL() string {
	env := GetEffectiveEnvironment()
	envConfig := GetEnvironmentConfig(env)

	if envConfig.OAuth.BaseURL != "" {
		return envConfig.OAuth.BaseURL
	}

	switch env {
	case EnvDevelopment:
		return DefaultDevelopmentURL
	default:
		return DefaultProductionURL
	}
}

// GetEffectiveClientID returns the OAuth client ID for the current environment.
func GetEffectiveClientID() string {
	env := GetEffectiveEnvironment()
	envConfig := GetEnvironmentConfig(env)

	if envConfig.OAuth.ClientID != "" {
		return envConfig.OAuth.ClientID
	}

	switch env {
	case EnvDevelopment:
		return DefaultDevelopmentClientID
	default:
		return DefaultProductionClientID
	}
}

// GetEffectiveScopes returns the OAuth scopes for the current environment.
func GetEffectiveScopes() []string {
	envConfig := GetEffectiveEnvironmentConfig()

	if len(envConfig.OAuth.Scopes) > 0 {
		return envConfig.OAuth.Scopes
	}

	return []string{"licenses:read"}
}

// GetEffectiveRedirectPath returns the OAuth redirect path for the current environment.
func GetEffectiveRedirectPath() string {
	envConfig := GetEffectiveEnvironmentConfig()

	if envConfig.OAuth.RedirectPath != "" {
		return envConfig.OAuth.RedirectPath
	}

	return "/customer-oauth/complete"
}

// IsNonInteractive checks if non-interactive mode is enabled.
func IsNonInteractive() bool {
	return GetFlags().NonInteractive
}

// IsVerbose checks if verbose mode is enabled.
func IsVerbose() bool {
	return GetFlags().Verbose
}

// ValidateEnvironment checks if an environment string is valid.
func ValidateEnvironment(env string) error {
	switch Environment(env) {
	case EnvProduction, EnvDevelopment:
		return nil
	default:
		return clierrors.Newf(clierrors.CodeInvalidInput, "invalid environment: %s (must be 'production' or 'development')", env)
	}
}

// Reset clears the cached configuration.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	current = nil
	flags = GlobalFlags{}
}
