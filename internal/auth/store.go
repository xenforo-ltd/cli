package auth

import (
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/xenforo-ltd/cli/internal/config"
)

// Store is the selected credential source. Environment credentials are read-only.
type Store interface {
	LoadToken() (*Token, error)
	SaveToken(*Token) error
	DeleteToken() error
	Source() string
}

// NewStore resolves environment credentials before the configured persistent store.
func NewStore() (Store, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load authentication configuration: %w", err)
	}

	return selectStore(cfg, os.Getenv("XF_TOKEN"), config.AuthFilePath, func() (Store, error) {
		return newKeychain(), nil
	})
}

// selectStore prefers a valid environment token over the configured file or keychain.
func selectStore(cfg config.Config, raw string, filePath func() (string, error), keychainStore func() (Store, error)) (Store, error) {
	if raw != "" {
		if strings.ContainsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
			return nil, fmt.Errorf("XF_TOKEN must be a raw access token without whitespace or control characters: %w", ErrInvalidInput)
		}

		return &environmentStore{token: Token{AccessToken: raw, TokenType: "Bearer", BaseURL: cfg.OAuth.BaseURL, External: true}}, nil
	}

	switch cfg.Auth.Storage {
	case "", "keychain":
		return keychainStore()
	case "file":
		path, err := filePath()
		if err != nil {
			return nil, fmt.Errorf("failed to locate authentication file: %w", err)
		}

		return &FileStore{path: path}, nil
	default:
		return nil, fmt.Errorf("auth.storage must be keychain or file: %w", ErrInvalidInput)
	}
}

// WritableStore supports the persistent authentication lifecycle.
// Logout may read an insecure regular file solely to revoke its credentials.
type WritableStore interface {
	Store
	PrepareLogin() error
	LoadTokenForLogout() (*Token, error)
}

// errExternallyManaged indicates that an environment token cannot be changed here.
var errExternallyManaged = fmt.Errorf("XF_TOKEN manages authentication externally; unset XF_TOKEN to manage stored credentials, or replace it with a new access token: %w", errUnsupported)

// NewWritableStore selects persistent storage without relying on display labels.
func NewWritableStore() (WritableStore, error) {
	store, err := NewStore()
	if err != nil {
		return nil, err
	}

	writable, ok := store.(WritableStore)
	if !ok {
		return nil, errExternallyManaged
	}

	return writable, nil
}

// environmentStore exposes a read-only token supplied through the environment.
type environmentStore struct{ token Token }

// Source returns the credential source name.
func (s *environmentStore) Source() string { return "environment" }

// LoadToken returns a copy of the environment token.
func (s *environmentStore) LoadToken() (*Token, error) {
	token := s.token
	return &token, nil
}

// SaveToken rejects changes to externally managed credentials.
func (s *environmentStore) SaveToken(*Token) error { return errExternallyManaged }

// DeleteToken rejects removal of externally managed credentials.
func (s *environmentStore) DeleteToken() error { return errExternallyManaged }

// RequireAuthFrom loads credentials and checks their configured destination.
func RequireAuthFrom(store Store) (*Token, error) {
	token, err := store.LoadToken()
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load authentication configuration: %w", err)
	}

	if token.BaseURL != cfg.OAuth.BaseURL {
		return nil, fmt.Errorf("authenticated for a different configuration - run 'xf auth login': %w", ErrConfigMismatch)
	}

	return token, nil
}
