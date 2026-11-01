package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	// keyringService is the service name used in the system keychain.
	keyringService = "xf"

	// keyringUser is the user/account name used in the system keychain.
	keyringUser = "oauth-token"
)

// Token represents an OAuth token with expiry information.
type Token struct {
	// External tokens have no local expiry or refresh metadata.
	External     bool      `json:"-"`
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

// keychain manages secure token storage in the system keychain.
type keychain struct{}

// newKeychain creates a new keychain instance.
func newKeychain() *keychain {
	return &keychain{}
}

// isAvailable checks if the system keychain is accessible.
func (k *keychain) isAvailable() bool {
	// Try to access the keychain by getting a non-existent key
	// If we get ErrNotFound, the keychain is available
	// If we get a different error, it's unavailable
	_, err := keyring.Get(keyringService, "__test_availability__")
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
func (k *keychain) SaveToken(token *Token) error {
	if token == nil || token.External {
		return fmt.Errorf("invalid token for keychain storage: %w", ErrInvalidInput)
	}

	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	if err := keyring.Set(keyringService, keyringUser, string(data)); err != nil {
		return fmt.Errorf("failed to save token to keychain: %w", err)
	}

	return nil
}

// LoadToken retrieves the stored token from the keychain.
func (k *keychain) LoadToken() (*Token, error) {
	data, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, fmt.Errorf("not authenticated - run 'xf auth login': %w", ErrAuthRequired)
		}

		return nil, keychainUnavailable(err)
	}

	var token Token
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, fmt.Errorf("failed to parse token from keychain: %w", err)
	}

	return &token, nil
}

// DeleteToken removes the token from the keychain.
func (k *keychain) DeleteToken() error {
	err := keyring.Delete(keyringService, keyringUser)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}

		return fmt.Errorf("failed to delete token from keychain: %w", err)
	}

	return nil
}

func (k *keychain) Source() string { return "keychain" }

func keychainUnavailable(cause error) error {
	if cause != nil {
		return fmt.Errorf("system keychain is unavailable (%v); Linux requires a running, unlocked Secret Service; select XF_AUTH_STORAGE=file or supply XF_TOKEN: %w", cause, ErrStoreUnavailable)
	}
	return fmt.Errorf("system keychain is unavailable (Linux requires a running, unlocked Secret Service); select XF_AUTH_STORAGE=file or supply XF_TOKEN: %w", ErrStoreUnavailable)
}

func (k *keychain) PrepareLogin() error {
	if !k.isAvailable() {
		return keychainUnavailable(nil)
	}
	return nil
}

func (k *keychain) LoadTokenForLogout() (*Token, error) { return k.LoadToken() }
