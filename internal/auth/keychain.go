package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/xenforo-ltd/cli/internal/config"

	"github.com/zalando/go-keyring"
)

const (
	// KeyringService is the service name used in the system keychain.
	KeyringService = "xf"

	// KeyringUser is the user/account name used in the system keychain.
	KeyringUser = "oauth-token"
)

// Token represents an OAuth token with expiry information.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
	IssuedAt     time.Time `json:"issued_at"`
	BaseURL      string    `json:"base_url"`
}

// IsExpired checks if the token has expired, accounting for clock skew.
func (t *Token) IsExpired() bool {
	// Consider expired 30 seconds early to account for clock skew
	return time.Now().Add(30 * time.Second).After(t.ExpiresAt)
}

// IsExpiringSoon checks if the token will expire within the given duration.
func (t *Token) IsExpiringSoon(within time.Duration) bool {
	return time.Now().Add(within).After(t.ExpiresAt)
}

// TimeUntilExpiry returns the duration until the token expires.
func (t *Token) TimeUntilExpiry() time.Duration {
	return time.Until(t.ExpiresAt)
}

// Keychain manages secure token storage in the system keychain.
type Keychain struct{}

// NewKeychain creates a new Keychain instance.
func NewKeychain() *Keychain {
	return &Keychain{}
}

// IsAvailable checks if the system keychain is accessible.
func (k *Keychain) IsAvailable() bool {
	// Try to access the keychain by getting a non-existent key
	// If we get ErrNotFound, the keychain is available
	// If we get a different error, it's unavailable
	_, err := keyring.Get(KeyringService, "__test_availability__")
	if errors.Is(err, keyring.ErrNotFound) {
		return true
	}
	// If no error, somehow this key exists (unlikely but fine)
	if err == nil {
		return true
	}

	return false
}

// SaveToken stores a token in the keychain.
func (k *Keychain) SaveToken(token *Token) error {
	if token == nil {
		return fmt.Errorf("token cannot be nil: %w", ErrInvalidInput)
	}

	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	if err := keyring.Set(KeyringService, KeyringUser, string(data)); err != nil {
		return fmt.Errorf("failed to save token to keychain: %w", err)
	}

	return nil
}

// LoadToken retrieves the stored token from the keychain.
func (k *Keychain) LoadToken() (*Token, error) {
	data, err := keyring.Get(KeyringService, KeyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, fmt.Errorf("not authenticated - run 'xf auth login': %w", err)
		}

		return nil, fmt.Errorf("failed to read token from keychain: %w", err)
	}

	var token Token
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, fmt.Errorf("failed to parse token from keychain: %w", err)
	}

	return &token, nil
}

// DeleteToken removes the token from the keychain.
func (k *Keychain) DeleteToken() error {
	err := keyring.Delete(KeyringService, KeyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("failed to delete token from keychain: %w", err)
	}

	return nil
}

// RequireAuth should be called at the start of commands that require authentication.
func RequireAuth() (*Token, error) {
	kc := NewKeychain()

	if !kc.IsAvailable() {
		return nil, fmt.Errorf("system keychain is not available - this is required for secure token storage: %w", ErrAuthRequired)
	}

	token, err := kc.LoadToken()
	if err != nil {
		return nil, err
	}

	// Check if token matches current configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load authentication configuration: %w", err)
	}

	if token.BaseURL != cfg.OAuth.BaseURL {
		return nil, fmt.Errorf("authenticated for a different configuration - run 'xf auth login': %w", ErrAuthRequired)
	}

	return token, nil
}
