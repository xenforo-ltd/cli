package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/config"
)

func TestCredentialSelection(t *testing.T) {
	for _, tc := range []struct {
		name, storage, raw, source string
		wantErr                    bool
		keyCalls, fileCalls        int
	}{
		{name: "default", source: "keychain", keyCalls: 1},
		{name: "keychain", storage: "keychain", source: "keychain", keyCalls: 1},
		{name: "file", storage: "file", source: "file", fileCalls: 1},
		{name: "environment before keychain", storage: "keychain", raw: "secret", source: "environment"},
		{name: "environment before file", storage: "file", raw: "secret", source: "environment"},
		{name: "environment before invalid configuration", storage: "bad", raw: "secret", source: "environment"},
		{name: "bad storage", storage: "bad", wantErr: true},
		{name: "whitespace", storage: "file", raw: "secret token", wantErr: true},
		{name: "newline", raw: "secret\n", wantErr: true},
		{name: "blank", raw: " ", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var keyCalls, fileCalls int
			cfg := config.Config{Auth: config.AuthConfig{Storage: tc.storage}, OAuth: config.OAuthConfig{BaseURL: "https://example.com"}}
			store, err := selectStore(cfg, tc.raw, func() (string, error) { fileCalls++; return "unused", nil }, func() (Store, error) { keyCalls++; return NewKeychain(), nil })
			if (err != nil) != tc.wantErr {
				t.Fatalf("selection error = %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("error exposes token")
			}
			if keyCalls != tc.keyCalls || fileCalls != tc.fileCalls {
				t.Fatalf("backend access: keychain=%d file=%d", keyCalls, fileCalls)
			}
			if err != nil {
				return
			}
			if store.Source() != tc.source {
				t.Fatalf("source=%s", store.Source())
			}
			if tc.source == "environment" {
				token, err := store.LoadToken()
				if err != nil || token.AccessToken != tc.raw || !token.External || token.BaseURL != cfg.OAuth.BaseURL || token.RefreshToken != "" {
					t.Fatalf("incorrect environment credentials: %v", err)
				}
				if _, ok := store.(WritableStore); ok {
					t.Fatal("environment store exposes writable lifecycle")
				}
				for _, err := range []error{store.SaveToken(token), store.DeleteToken()} {
					if !errors.Is(err, ErrUnsupported) {
						t.Fatalf("expected externally managed error: %v", err)
					}
				}
				token.AccessToken = "mutated"
				again, _ := store.LoadToken()
				if again.AccessToken != tc.raw {
					t.Fatal("environment credentials mutated")
				}
			}
		})
	}
}

func TestUnavailableKeychainDoesNotFallBack(t *testing.T) {
	unavailable := errors.New("keychain unavailable")
	_, err := selectStore(config.Config{}, "", func() (string, error) { t.Fatal("unexpected file fallback"); return "", nil }, func() (Store, error) { return nil, unavailable })
	if !errors.Is(err, unavailable) {
		t.Fatalf("error = %v", err)
	}
}
