package auth

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	// keyringService is the service name used in the system keychain.
	keyringService = "xf"

	// keyringUser is the user/account name used in the system keychain.
	keyringUser = "oauth-token"
)

// keychain manages secure token storage in the system keychain.
type keychain struct{}

// newKeychain creates a new keychain instance.
func newKeychain() *keychain {
	return &keychain{}
}

// isAvailable probes the keychain and treats a missing probe key as available.
func (k *keychain) isAvailable() bool {
	_, err := keyring.Get(keyringService, "__test_availability__")

	return err == nil || errors.Is(err, keyring.ErrNotFound)
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
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("failed to delete token from keychain: %w", err)
	}

	return nil
}

// Source returns the credential source name.
func (k *keychain) Source() string { return "keychain" }

// keychainUnavailable describes an unavailable keychain and alternatives.
func keychainUnavailable(cause error) error {
	if cause != nil {
		return fmt.Errorf("system keychain is unavailable (%v); Linux requires a running, unlocked Secret Service; select XF_AUTH_STORAGE=file or supply XF_TOKEN: %w", cause, ErrStoreUnavailable)
	}

	return fmt.Errorf("system keychain is unavailable (Linux requires a running, unlocked Secret Service); select XF_AUTH_STORAGE=file or supply XF_TOKEN: %w", ErrStoreUnavailable)
}

// PrepareLogin checks keychain access before starting authentication.
func (k *keychain) PrepareLogin() error {
	if !k.isAvailable() {
		return keychainUnavailable(nil)
	}

	return nil
}

// LoadTokenForLogout retrieves credentials for revocation.
func (k *keychain) LoadTokenForLogout() (*Token, error) { return k.LoadToken() }
