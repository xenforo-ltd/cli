// Package config manages CLI configuration and settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/viper"
)

var (
	// cacheOnce limits configuration unmarshalling to the first Load call.
	cacheOnce sync.Once
	// cache holds the configuration unmarshalled by the first Load call.
	cache Config
	// errCache holds the error from the first configuration unmarshalling.
	errCache error

	// initMu guards Init, which configures viper's package-level singleton.
	// Concurrent callers — notably parallel tests that each run the CLI —
	// would otherwise race on that shared state.
	initMu sync.Mutex
)

// Config holds CLI settings for output, interaction, cache, credential storage,
// and OAuth endpoints and credentials.
type Config struct {
	Verbose bool `json:"verbose" mapstructure:"verbose"`

	NoInteraction bool `json:"no_interaction" mapstructure:"no_interaction"`

	CachePath string `json:"cache_path" mapstructure:"cache_path"`

	Auth authConfig `json:"auth" mapstructure:"auth"`

	OAuth OAuthConfig `json:"oauth" mapstructure:"oauth"`
}

// authConfig holds persistent credential storage settings.
type authConfig struct {
	Storage string `json:"storage" mapstructure:"storage"`
}

// AuthFilePath returns the credential file beside the configuration file.
func AuthFilePath() (string, error) {
	if path := viper.ConfigFileUsed(); path != "" {
		return filepath.Abs(filepath.Join(filepath.Dir(path), "auth.json"))
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "xf", "auth.json"), nil
}

// OAuthConfig holds the OAuth base URL, client ID, requested scopes, and local
// callback path.
type OAuthConfig struct {
	BaseURL string `json:"base_url" mapstructure:"base_url"`

	ClientID string `json:"client_id" mapstructure:"client_id"`

	Scopes []string `json:"scopes" mapstructure:"scopes"`

	RedirectPath string `json:"redirect_path" mapstructure:"redirect_path"`
}

// oauthEndpoints holds the OAuth endpoint URLs.
type oauthEndpoints struct {
	Auth       string
	Token      string
	Introspect string
	Revoke     string
}

// Endpoints returns the OAuth endpoint URLs.
func (cfg *OAuthConfig) Endpoints() *oauthEndpoints {
	base := strings.TrimSuffix(cfg.BaseURL, "/")

	return &oauthEndpoints{
		Auth:       base + "/customer-oauth/authorize",
		Token:      base + "/api/customer-oauth2/token",
		Introspect: base + "/api/customer-oauth2/introspect",
		Revoke:     base + "/api/customer-oauth2/revoke",
	}
}

// Init configures Viper defaults and environment bindings and reads the config
// file. It serializes access to Viper and returns file-reading errors.
func Init(configFile string) error {
	initMu.Lock()
	defer initMu.Unlock()

	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return fmt.Errorf("could not determine user config directory: %w", err)
		}

		viper.AddConfigPath(filepath.Join(configDir, "xf"))
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return fmt.Errorf("could not determine user cache directory: %w", err)
	}

	viper.AllowEmptyEnv(true)
	viper.SetEnvPrefix("xf")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("auth.storage", "keychain")
	viper.SetDefault("verbose", false)
	viper.SetDefault("no_interaction", false)
	viper.SetDefault("cache_path", filepath.Join(cacheDir, "xf"))

	viper.SetDefault("oauth.base_url", "https://xenforo.com/")
	viper.SetDefault("oauth.client_id", "5062897895166491")
	viper.SetDefault("oauth.scopes", []string{"licenses:read"})
	viper.SetDefault("oauth.redirect_path", "/customer-oauth/complete")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	return nil
}

// Load unmarshals configuration once and returns the cached result. It shares
// Init's lock to protect Viper's package-level state during unmarshalling.
func Load() (Config, error) {
	cacheOnce.Do(func() {
		initMu.Lock()
		defer initMu.Unlock()

		if err := viper.Unmarshal(&cache); err != nil {
			errCache = fmt.Errorf("failed to unmarshal config: %w", err)
		}
	})

	return cache, errCache
}
