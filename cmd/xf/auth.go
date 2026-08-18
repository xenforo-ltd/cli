package main

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
Tokens are stored securely in your system keychain.`,
	Example: `  # Log in to your XenForo account
  xf auth login

  # Check current authentication status
  xf auth status

  # Log out and revoke tokens
  xf auth logout`,
	// NoArgs rejects an unknown subcommand with cobra's own error. RunE is
	// required alongside it: without a RunE, cobra skips a parent's Args
	// validator entirely and silently prints help instead.
	Args:    cobra.NoArgs,
	GroupID: "start",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with XenForo",
	Long: `Start the OAuth authentication flow to log in to your XenForo customer account.

This will open your browser to complete authentication. The CLI will automatically
receive the authorization when you complete the login. Tokens are stored securely
in your system keychain.`,
	Example: `  # Standard login (opens browser)
  xf auth login

  # Login with custom timeout
  xf auth login --timeout 600`,
	Args: cobra.NoArgs,
	RunE: runAuthLogin,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	Long: `Display the current authentication status, including token validity.

This command shows whether you're authenticated, token expiration time,
and performs server-side validation to ensure the token is still active.`,
	Example: `  # Show authentication status
  xf auth status

  # Output as JSON (useful for scripts)
  xf auth status --json`,
	Args: cobra.NoArgs,
	RunE: runAuthStatus,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out and revoke tokens",
	Long:  `Log out by revoking the access and refresh tokens and removing them from the system keychain.`,
	Example: `  # Log out
  xf auth logout`,
	Args: cobra.NoArgs,
	RunE: runAuthLogout,
}

var authRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh the access token",
	Long: `Manually refresh the access token using the stored refresh token.

Normally, tokens are refreshed automatically when needed. Use this command
to manually refresh before the token expires.`,
	Example: `  # Manually refresh token
  xf auth refresh`,
	Args: cobra.NoArgs,
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

// errKeychainUnavailable is the shared error for every auth subcommand that
// requires the system keychain but finds it unavailable.
func errKeychainUnavailable() error {
	return withHint(
		markAs(ErrKeychainUnavailable, "the system keychain is not available"),
		"Tokens are stored in the keychain; unlock or enable it and try again",
	)
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return errKeychainUnavailable()
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

	ui.PrintInfo("If the browser does not open, visit:")
	ui.Printf("%s%s\n", ui.Indent1, ui.URL.Render(authURL))
	ui.Println()

	spinner := ui.NewSpinner("Opening browser for authentication")
	spinner.Start()

	if err := auth.OpenBrowser(cmd.Context(), authURL); err != nil {
		ui.PrintWarning("Could not open the browser automatically — use the URL above")
	}

	spinner.UpdateMessage("Waiting for authentication in the browser")

	ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(flagAuthTimeout)*time.Second)
	defer cancel()

	result, err := callbackServer.WaitForCallback(ctx)
	if err != nil {
		spinner.Stop()
		return fmt.Errorf("failed to wait for authentication callback: %w", err)
	}

	if result.Error != "" {
		spinner.Stop()
		return fmt.Errorf("authentication failed: %s: %w", result.Error, ErrAuthFailed)
	}

	if result.State != pkce.State {
		spinner.Stop()
		return fmt.Errorf("authentication failed: state mismatch (possible CSRF attack): %w", ErrAuthFailed)
	}

	spinner.UpdateMessage("Completing authentication")

	token, err := client.ExchangeCode(ctx, result.Code, pkce, redirectURI)
	if err != nil {
		spinner.Stop()
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	if err := kc.SaveToken(token); err != nil {
		spinner.Stop()
		return fmt.Errorf("failed to save authentication token: %w", err)
	}

	message := "Authentication complete"

	ctx2, cancel2 := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel2()

	if introspect, err := client.IntrospectToken(ctx2, token.AccessToken); err == nil && introspect.Username != "" {
		message = "Authenticated as " + ui.Bold.Render(introspect.Username)
	}

	spinner.StopWithMessage("success", message)

	return nil
}

// authStatusJSON is the single stable shape for `xf auth status --json`,
// regardless of whether the keychain is unavailable, no token is stored, or
// a token is present (valid or expired).
type authStatusJSON struct {
	Authenticated bool   `json:"authenticated"`
	Expired       bool   `json:"expired"`
	Scope         string `json:"scope,omitempty"`
	IssuedAt      string `json:"issued_at,omitempty"`  // RFC3339
	ExpiresAt     string `json:"expires_at,omitempty"` // RFC3339
	ServerValid   *bool  `json:"server_valid,omitempty"`
	Username      string `json:"username,omitempty"`
	Error         string `json:"error,omitempty"`
}

func printAuthStatusJSON(output authStatusJSON) error {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal auth status: %w", err)
	}

	fmt.Println(string(data))

	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		if flagAuthStatusJSON {
			return printAuthStatusJSON(authStatusJSON{Error: "keychain unavailable"})
		}

		ui.PrintWarning("Not authenticated (keychain unavailable)")

		return nil
	}

	token, err := kc.LoadToken()
	if err != nil {
		if errors.Is(err, auth.ErrAuthRequired) {
			if flagAuthStatusJSON {
				return printAuthStatusJSON(authStatusJSON{})
			}

			ui.PrintInfo("Not authenticated")
			ui.PrintHint("Run " + ui.Command.Render("xf auth login") + " to authenticate")

			return nil
		}

		return fmt.Errorf("failed to load authentication token: %w", err)
	}

	expired := token.IsExpired()
	refreshable := token.RefreshToken != ""

	var (
		serverValid *bool
		username    string
	)

	if !expired {
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
		return printAuthStatusJSON(authStatusJSON{
			Authenticated: true,
			Expired:       expired,
			Scope:         token.Scope,
			IssuedAt:      token.IssuedAt.Format(time.RFC3339),
			ExpiresAt:     token.ExpiresAt.Format(time.RFC3339),
			ServerValid:   serverValid,
			Username:      username,
		})
	}

	switch {
	case expired && refreshable:
		ui.PrintWarning("Authenticated — token expired (will refresh automatically)")
	case expired:
		ui.PrintError("Authenticated — token expired")
		ui.PrintHint("Run " + ui.Command.Render("xf auth login") + " to re-authenticate")
	default:
		ui.PrintSuccess("Authenticated")
	}

	var expiresValue string

	if expired {
		expiresValue = ui.Warning.Render(ui.FormatDateTime(token.ExpiresAt) + " (expired)")
	} else {
		remaining := time.Until(token.ExpiresAt).Round(time.Minute)
		expiresValue = fmt.Sprintf("%s (in %s)", ui.FormatDateTime(token.ExpiresAt), remaining)
	}

	pairs := make([]ui.KVPair, 0, 5)

	if username != "" {
		pairs = append(pairs, ui.KV("User", username))
	}

	pairs = append(pairs,
		ui.KV("Scope", token.Scope),
		ui.KV("Issued", ui.FormatDateTime(token.IssuedAt)),
		ui.KV("Expires", expiresValue),
	)

	if serverValid != nil {
		serverValue := ui.Success.Render("Active")
		if !*serverValid {
			serverValue = ui.Error.Render("Revoked or invalid")
		}

		pairs = append(pairs, ui.KV("Server validation", serverValue))
	}

	ui.Println()
	ui.PrintKeyValuePadded(pairs)

	return nil
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return errKeychainUnavailable()
	}

	token, err := kc.LoadToken()
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

	ui.SuccessBox("Logged out", nil)

	return nil
}

func runAuthRefresh(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return errKeychainUnavailable()
	}

	token, err := kc.LoadToken()
	if err != nil {
		return fmt.Errorf("failed to load authentication token: %w", err)
	}

	if token.RefreshToken == "" {
		return withHint(errors.New("no refresh token available"), "Run "+ui.Command.Render("xf auth login")+" to authenticate")
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

	ui.PrintSuccess("Token refreshed")
	ui.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("New expiry", newToken.ExpiresAt.Format(time.RFC1123)),
		ui.KV("Time until expiry", newToken.TimeUntilExpiry().Round(time.Minute).String()),
	})

	return nil
}
