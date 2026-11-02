package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// Refresh exchanges the stored refresh token for new credentials and saves them.
func Refresh(ctx context.Context) error {
	store, err := auth.NewWritableStore()
	if err != nil {
		return err
	}

	token, err := auth.RequireAuthFrom(store)
	if err != nil {
		return fmt.Errorf("failed to load authentication token: %w", err)
	}

	if token.RefreshToken == "" {
		return clierror.WithHint(errors.New("no refresh token available"), "Run "+ui.Command.Render("xf auth login")+" to authenticate")
	}

	ui.PrintInfo("Refreshing access token")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load auth configuration: %w", err)
	}

	client := auth.NewOAuthClient(&config.OAuthConfig{
		BaseURL:  token.BaseURL,
		ClientID: cfg.OAuth.ClientID,
	})

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	newToken, err := client.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	if err := store.SaveToken(newToken); err != nil {
		return fmt.Errorf("failed to save refreshed authentication token: %w", err)
	}

	ui.PrintSuccess("Token refreshed")
	ui.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("New expiry", ui.FormatDateTime(newToken.ExpiresAt)),
		ui.KV("Time until expiry", newToken.TimeUntilExpiry().Round(time.Minute).String()),
	})

	return nil
}
