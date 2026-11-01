package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func Logout(ctx context.Context) error {
	store, err := auth.NewWritableStore()
	if err != nil {
		return err
	}

	token, err := store.LoadTokenForLogout()
	if err != nil {
		if errors.Is(err, auth.ErrAuthRequired) {
			ui.PrintInfo("Already logged out")
			return nil
		}

		return fmt.Errorf("failed to load authentication token: %w", err)
	}

	client := auth.NewOAuthClient(&config.OAuthConfig{
		BaseURL: token.BaseURL,
	})

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load auth configuration: %w", err)
	}

	revocations := []struct{ label, token string }{{"token", token.AccessToken}}
	if token.RefreshToken != "" {
		revocations = append(revocations, struct{ label, token string }{"refresh token", token.RefreshToken})
	}

	for _, r := range revocations {
		if err := client.RevokeToken(ctx, r.token); err != nil && cfg.Verbose {
			ui.PrintWarning(fmt.Sprintf("Could not revoke %s on server: %v", r.label, err))
		}
	}

	if err := store.DeleteToken(); err != nil {
		return fmt.Errorf("failed to delete stored authentication token: %w", err)
	}

	ui.SuccessBox("Logged out", nil)

	return nil
}
