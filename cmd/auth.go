package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
	Long: `Manage OAuth authentication with XenForo customer area.

Authentication is required to download XenForo packages and access your licenses.
Tokens are stored securely in your system keychain.

Examples:
  # Log in to your XenForo account
  xf auth login

  # Check current authentication status
  xf auth status

  # Log out and revoke tokens
  xf auth logout`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with XenForo",
	Long: `Start the OAuth authentication flow to log in to your XenForo customer account.

This will open your browser to complete authentication. The CLI will automatically
receive the authorization when you complete the login. Tokens are stored securely
in your system keychain.

	Examples:
	  # Standard login (opens browser)
	  xf auth login

	  # Login with custom timeout
	  xf auth login --timeout 600`,
	RunE: runAuthLogin,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	Long: `Display the current authentication status, including token validity.

This command shows whether you're authenticated, token expiration time,
and performs server-side validation to ensure the token is still active.

Examples:
  # Show authentication status
  xf auth status

  # Output as JSON (useful for scripts)
  xf auth status --json`,
	RunE: runAuthStatus,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and revoke tokens",
	Long: `Revoke the current OAuth tokens and remove them from the keychain.

This command will:
  1. Revoke the access token on the server
  2. Revoke the refresh token on the server (if present)
  3. Remove tokens from your system keychain

Examples:
  # Log out
  xf auth logout`,
	RunE: runAuthLogout,
}

var authRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh the access token",
	Long: `Manually refresh the access token using the stored refresh token.

Normally, tokens are refreshed automatically when needed. Use this command
to manually refresh before the token expires.

Examples:
  # Manually refresh token
  xf auth refresh`,
	RunE: runAuthRefresh,
}

var (
	flagAuthStatusJSON bool
	flagAuthTimeout    int
)

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authRefreshCmd)

	defaultTimeout := 5 * time.Minute

	authStatusCmd.Flags().BoolVar(&flagAuthStatusJSON, "json", false, "output as JSON")
	authLoginCmd.Flags().IntVar(&flagAuthTimeout, "timeout", int(defaultTimeout/time.Second), "timeout in seconds for browser authentication")

	rootCmd.AddCommand(authCmd)
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return fmt.Errorf("system keychain is not available - this is required for secure token storage: %w", ErrKeychainUnavailable)
	}

	pkce, err := auth.GeneratePKCE()
	if err != nil {
		return fmt.Errorf("failed to generate PKCE parameters: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load auth configuration: %w", err)
	}

	client := auth.NewOAuthClient(&cfg.OAuth)

	callbackServer, err := auth.NewCallbackServer(cmd.Context(), cfg.OAuth.RedirectPath)
	if err != nil {
		return fmt.Errorf("failed to start OAuth callback server: %w", err)
	}

	callbackServer.Start()

	defer func() {
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()

		_ = callbackServer.Shutdown(ctx)
	}()

	redirectURI := callbackServer.RedirectURI()
	authURL := client.AuthorizationURL(pkce, redirectURI)

	ui.PrintInfo("Opening browser for authentication...")
	ui.PrintInfo(fmt.Sprintf("If the browser doesn't open, visit this URL:\n%s\n\n", ui.URL.Render(authURL)))

	if err := auth.OpenBrowser(cmd.Context(), authURL); err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not open browser automatically: %v", err))
	}

	ui.PrintInfo("Waiting for authentication...")

	ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(flagAuthTimeout)*time.Second)
	defer cancel()

	result, err := callbackServer.WaitForCallback(ctx)
	if err != nil {
		return fmt.Errorf("failed to wait for authentication callback: %w", err)
	}

	if result.Error != "" {
		return fmt.Errorf("authentication failed: %s: %w", result.Error, ErrAuthFailed)
	}

	if result.State != pkce.State {
		return fmt.Errorf("authentication failed: state mismatch (possible CSRF attack): %w", ErrAuthFailed)
	}

	ui.PrintInfo("Exchanging authorization code for tokens...")

	token, err := client.ExchangeCode(ctx, result.Code, pkce, redirectURI)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	if err := kc.SaveToken(token); err != nil {
		return fmt.Errorf("failed to save authentication token: %w", err)
	}

	ui.PrintSuccess("Authentication successful!")

	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		if flagAuthStatusJSON {
			data, err := json.Marshal(map[string]any{
				"authenticated": false,
				"error":         "keychain unavailable",
			})
			if err != nil {
				return fmt.Errorf("failed to marshal auth status: %w", err)
			}

			ui.Println(string(data))

			return nil
		}

		ui.PrintWarning("Not authenticated (keychain unavailable)")

		return nil
	}

	token, err := kc.LoadToken()
	if err != nil {
		if errors.Is(err, auth.ErrAuthRequired) {
			if flagAuthStatusJSON {
				data, err := json.Marshal(map[string]any{
					"authenticated": false,
				})
				if err != nil {
					return fmt.Errorf("failed to marshal auth status: %w", err)
				}

				ui.Println(string(data))

				return nil
			}

			ui.PrintWarning("Not authenticated")
			ui.Printf("Run %s to authenticate.\n", ui.Command.Render("xf auth login"))

			return nil
		}

		return fmt.Errorf("failed to load authentication token: %w", err)
	}

	var (
		serverValid *bool
		username    string
	)

	if !token.IsExpired() {
		client := auth.NewOAuthClient(&config.OAuthConfig{
			BaseURL: token.BaseURL,
		})

		ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
		defer cancel()

		introspect, err := client.IntrospectToken(ctx, token.AccessToken)
		if err == nil {
			serverValid = &introspect.Active
			username = introspect.Username
		}
	}

	if flagAuthStatusJSON {
		output := map[string]any{
			"authenticated": true,
			"scope":         token.Scope,
			"expires_at":    token.ExpiresAt.Format(time.RFC3339),
			"issued_at":     token.IssuedAt.Format(time.RFC3339),
			"expired":       token.IsExpired(),
		}
		if serverValid != nil {
			output["server_valid"] = *serverValid
		}

		if username != "" {
			output["username"] = username
		}

		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal auth status: %w", err)
		}

		ui.Println(string(data))

		return nil
	}

	ui.PrintSuccess("Authenticated")
	ui.Println()

	var pairs []ui.KVPair
	if username != "" {
		pairs = append(pairs, ui.KV("User", username))
	}

	pairs = append(pairs, ui.KV("Scope", token.Scope))
	pairs = append(pairs, ui.KV("Issued", token.IssuedAt.Format(time.RFC1123)))
	pairs = append(pairs, ui.KV("Expires", token.ExpiresAt.Format(time.RFC1123)))
	ui.PrintKeyValuePadded(pairs)

	ui.Println()

	if token.IsExpired() {
		ui.Printf("%s %s\n", ui.StatusIcon("error"), ui.Error.Render("Token EXPIRED"))

		if token.RefreshToken != "" {
			ui.PrintDetail("Token can be refreshed automatically")
		}
	} else {
		remaining := token.TimeUntilExpiry().Round(time.Minute)
		ui.Printf("%s Token valid (%s remaining)\n", ui.StatusIcon("success"), ui.Success.Render(remaining.String()))
	}

	if serverValid != nil {
		if *serverValid {
			ui.Printf("%s Server validation: %s\n", ui.StatusIcon("success"), ui.Success.Render("Active"))
		} else {
			ui.Printf("%s Server validation: %s\n", ui.StatusIcon("error"), ui.Error.Render("Revoked or Invalid"))
		}
	}

	return nil
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return fmt.Errorf("system keychain is not available: %w", ErrKeychainUnavailable)
	}

	token, err := kc.LoadToken()
	if err != nil {
		if errors.Is(err, auth.ErrAuthRequired) {
			ui.PrintInfo("Already logged out.")
			return nil
		}

		return fmt.Errorf("failed to load authentication token: %w", err)
	}

	client := auth.NewOAuthClient(&config.OAuthConfig{
		BaseURL: token.BaseURL,
	})

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load auth configuration: %w", err)
	}

	if err := client.RevokeToken(ctx, token.AccessToken); err != nil {
		if cfg.Verbose {
			ui.PrintWarning(fmt.Sprintf("Could not revoke token on server: %v", err))
		}
	}

	if token.RefreshToken != "" {
		if err := client.RevokeToken(ctx, token.RefreshToken); err != nil {
			if cfg.Verbose {
				ui.PrintWarning(fmt.Sprintf("Could not revoke refresh token on server: %v", err))
			}
		}
	}

	if err := kc.DeleteToken(); err != nil {
		return fmt.Errorf("failed to delete authentication token: %w", err)
	}

	ui.PrintSuccess("Logged out successfully.")

	return nil
}

func runAuthRefresh(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return fmt.Errorf("system keychain is not available: %w", ErrKeychainUnavailable)
	}

	token, err := kc.LoadToken()
	if err != nil {
		return fmt.Errorf("failed to load authentication token: %w", err)
	}

	if token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available - run 'xf auth login': %w", ErrAuthFailed)
	}

	ui.PrintInfo("Refreshing access token...")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load auth configuration: %w", err)
	}

	client := auth.NewOAuthClient(&config.OAuthConfig{
		BaseURL:  token.BaseURL,
		ClientID: cfg.OAuth.ClientID,
	})

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	newToken, err := client.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	if err := kc.SaveToken(newToken); err != nil {
		return fmt.Errorf("failed to save refreshed authentication token: %w", err)
	}

	ui.PrintSuccess("Token refreshed successfully!")
	ui.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("New expiry", newToken.ExpiresAt.Format(time.RFC1123)),
		ui.KV("Time until expiry", newToken.TimeUntilExpiry().Round(time.Minute).String()),
	})

	return nil
}
