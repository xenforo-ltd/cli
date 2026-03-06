// Package config manages CLI configuration and settings.
package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

var (
	cacheOnce sync.Once
	cache     Config
	cacheErr  error
)

// Config holds all CLI configuration values.
type Config struct {
	// Verbose enables detailed output.
	Verbose bool `json:"verbose" mapstructure:"verbose"`

	// NoInteraction disables interactive prompts.
	NoInteraction bool `json:"no_interaction" mapstructure:"no_interaction"`

	// CachePath is the directory for cached downloads.
	CachePath string `json:"cache_path" mapstructure:"cache_path"`

	// OAuth holds OAuth-related settings.
	OAuth OAuthConfig `json:"oauth" mapstructure:"oauth"`
}

// OAuthConfig holds OAuth endpoint and client configuration.
type OAuthConfig struct {
	// BaseURL is the base URL for OAuth endpoints.
	BaseURL string `json:"base_url" mapstructure:"base_url"`

	// ClientID is the OAuth client identifier.
	ClientID string `json:"client_id" mapstructure:"client_id"`

	// Scopes are the OAuth scopes to request.
	Scopes []string `json:"scopes" mapstructure:"scopes"`

	// RedirectPath is the local callback path for the OAuth flow.
	RedirectPath string `json:"redirect_path" mapstructure:"redirect_path"`
}

// OAuthEndpoints holds the OAuth endpoint URLs.
type OAuthEndpoints struct {
	Auth       string
	Token      string
	Introspect string
	Revoke     string
}

// Endpoints returns the OAuth endpoint URLs.
func (cfg *OAuthConfig) Endpoints() *OAuthEndpoints {
	base := strings.TrimSuffix(cfg.BaseURL, "/")

	return &OAuthEndpoints{
		Auth:       base + "/customer-oauth/authorize",
		Token:      base + "/api/customer-oauth2/token",
		Introspect: base + "/api/customer-oauth2/introspect",
		Revoke:     base + "/api/customer-oauth2/revoke",
	}
}

// Init sets up the configuration system and reads the config file if it exists.
func Init(configFile string) error {
	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return clierrors.Wrap(clierrors.CodeConfigReadFailed, "could not determine user config directory", err)
		}

		viper.AddConfigPath(filepath.Join(configDir, "xf"))
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return clierrors.Wrap(clierrors.CodeConfigReadFailed, "could not determine user cache directory", err)
	}

	viper.AllowEmptyEnv(true)
	viper.SetEnvPrefix("xf")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("verbose", false)
	viper.SetDefault("no_interaction", false)
	viper.SetDefault("cache_path", filepath.Join(cacheDir, "xf"))

	viper.SetDefault("oauth.base_url", "https://xenforo.com/")
	viper.SetDefault("oauth.client_id", "5062897895166491")
	viper.SetDefault("oauth.scopes", []string{"licenses:read"})
	viper.SetDefault("oauth.redirect_path", "/customer-oauth/complete")

	if err := viper.ReadInConfig(); err != nil {
		return clierrors.Wrap(clierrors.CodeConfigReadFailed, "failed to read config file", err)
	}

	return nil
}

// Load reads the configuration from the config file.
func Load() (Config, error) {
	cacheOnce.Do(func() {
		if err := viper.Unmarshal(&cache); err != nil {
			cacheErr = clierrors.Wrap(clierrors.CodeConfigInvalid, "failed to unmarshal config", err)
		}
	})

	return cache, cacheErr
}

// Save writes the current configuration to the config file.
func Save() error {
	return viper.WriteConfig()
}
