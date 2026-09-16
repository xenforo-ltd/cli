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
Tokens use your configured keychain or file store. XF_TOKEN overrides stored credentials.`,
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
receive the authorization when you complete the login. Keychain tokens are stored
securely. File-store tokens are plaintext, protected only by filesystem permissions.`,
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
	Long:  `Log out by revoking the access and refresh tokens and removing them from the configured credential store.`,
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

func runAuthLogin(cmd *cobra.Command, args []string) error {
	store, err := auth.NewWritableStore()
	if err != nil {
		return err
	}

	if err := store.PrepareLogin(); err != nil {
		return err
	}
	if file, ok := store.(*auth.FileStore); ok {
		ui.PrintInfo("Credential file: " + file.Path() + " (plaintext; keep out of version control)")
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

	if err := store.SaveToken(token); err != nil {
		return fmt.Errorf("failed to save authentication token: %w", err)
	}

	ui.PrintSuccess("Authentication successful!")
	return nil
}

// authStatusJSON is the single stable shape for `xf auth status --json`,
// regardless of whether the keychain is unavailable, no token is stored, or
// a token is present (valid or expired).
// Authenticated and Expired are pointers so an unknown value can be reported
// as null rather than being coerced into a definite false.
type authStatusJSON struct {
	Authenticated *bool  `json:"authenticated"`
	Source        string `json:"source,omitempty"`
	StoragePath   string `json:"storage_path,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Expired       *bool  `json:"expired"`
	Scope         string `json:"scope,omitempty"`
	IssuedAt      string `json:"issued_at,omitempty"`  // RFC3339
	ExpiresAt     string `json:"expires_at,omitempty"` // RFC3339
	ServerValid   *bool  `json:"server_valid,omitempty"`
	Username      string `json:"username,omitempty"`
	Error         string `json:"error,omitempty"`
}

// environmentAuthStatusJSON is the shape of `xf auth status --json` when the
// token comes from XF_TOKEN.
//
// Unlike authStatusJSON, whose optional fields are absent when there is no
// stored token, the environment contract reports a fixed set of keys. Scripts
// for XF_TOKEN branch on their presence, so the pointer fields carry no
// omitempty and unknown values serialize as explicit null. Scope and username
// only exist once introspection has succeeded, matching the original
// map-based output.
type environmentAuthStatusJSON struct {
	Authenticated *bool   `json:"authenticated"`
	Source        string  `json:"source"`
	ServerValid   *bool   `json:"server_valid"`
	ExpiresAt     *string `json:"expires_at"`
	IssuedAt      *string `json:"issued_at"`
	Expired       *bool   `json:"expired"`
	Scope         *string `json:"scope,omitempty"`
	Username      *string `json:"username,omitempty"`
	Error         string  `json:"error,omitempty"`
}

func printAuthStatusJSON(output any) error {
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal auth status: %w", err)
	}

	fmt.Println(string(data))

	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	store, err := auth.NewStore()
	if err != nil {
		return reportAuthUnavailable(store, err)
	}

	token, err := auth.RequireAuthFrom(store)
	if err != nil {
		return reportAuthUnavailable(store, err)
	}

	if token.External {
		return runEnvironmentAuthStatus(cmd, token)
	}

	expired := token.IsExpired()
	refreshable := token.RefreshToken != ""

	var (
		serverValid *bool
		username    string
	)

	if !expired {
		introspect, err := introspectAuthToken(cmd.Context(), token)
		if err == nil {
			serverValid = &introspect.Active
			username = introspect.Username
		}
	}

	if flagAuthStatusJSON {
		output := authStatusJSON{
			Authenticated: new(true),
			Source:        store.Source(),
			Expired:       new(expired),
			Scope:         token.Scope,
			IssuedAt:      token.IssuedAt.Format(time.RFC3339),
			ExpiresAt:     token.ExpiresAt.Format(time.RFC3339),
			ServerValid:   serverValid,
			Username:      username,
		}
		if file, ok := store.(*auth.FileStore); ok {
			output.StoragePath = file.Path()
		}

		return printAuthStatusJSON(output)
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

	pairs := make([]ui.KVPair, 0, 7)

	pairs = append(pairs, ui.KV("Source", store.Source()))
	if file, ok := store.(*auth.FileStore); ok {
		pairs = append(pairs, ui.KV("File", ui.Path.Render(file.Path())))
	}

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

	if err := store.DeleteToken(); err != nil {
		return fmt.Errorf("failed to delete stored authentication token: %w", err)
	}

	ui.SuccessBox("Logged out", nil)

	return nil
}

func runAuthRefresh(cmd *cobra.Command, args []string) error {
	store, err := auth.NewWritableStore()
	if err != nil {
		return err
	}

	token, err := auth.RequireAuthFrom(store)
	if err != nil {
		return fmt.Errorf("failed to load authentication token: %w", err)
	}

	if token.RefreshToken == "" {
		return withHint(errors.New("no refresh token available"), "Run "+ui.Command.Render("xf auth login")+" to authenticate")
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

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
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

func reportAuthUnavailable(store auth.Store, err error) error {
	reason := "storage_error"
	switch {
	case errors.Is(err, auth.ErrAuthRequired):
		reason = "not_authenticated"
	case errors.Is(err, auth.ErrStoreUnavailable):
		reason = "store_unavailable"
	case errors.Is(err, auth.ErrConfigMismatch):
		reason = "configuration_mismatch"
	case errors.Is(err, auth.ErrInvalidInput):
		reason = "invalid_credentials"
	}
	if flagAuthStatusJSON {
		output := authStatusJSON{Authenticated: new(false), Reason: reason}
		if store != nil {
			output.Source = store.Source()
		}
		if reason != "not_authenticated" {
			output.Error = auth.ErrorMessage(err)
		}

		return printAuthStatusJSON(output)
	}
	if errors.Is(err, auth.ErrAuthRequired) || errors.Is(err, auth.ErrConfigMismatch) {
		ui.PrintWarning(auth.ErrorMessage(err))
		return nil
	}
	return err
}

func runEnvironmentAuthStatus(cmd *cobra.Command, token *auth.Token) error {
	result, validationErr := introspectAuthToken(cmd.Context(), token)
	// Every established key is emitted; unknown ones stay nil and serialize
	// as explicit null so scripts can rely on their presence.
	output := environmentAuthStatusJSON{Source: "environment"}
	if validationErr == nil {
		output.Authenticated = new(result.Active)
		output.ServerValid = new(result.Active)

		// Introspection succeeded, so the claims exist even when empty.
		scope := result.Scope
		username := result.Username
		output.Scope = &scope
		output.Username = &username

		if result.Exp > 0 {
			expiry := time.Unix(result.Exp, 0)
			expiresAt := expiry.Format(time.RFC3339)
			output.ExpiresAt = &expiresAt
			output.Expired = new(!time.Now().Before(expiry))
		}
		if result.Iat > 0 {
			issuedAt := time.Unix(result.Iat, 0).Format(time.RFC3339)
			output.IssuedAt = &issuedAt
		}
	} else {
		output.Error = "Unable to validate XF_TOKEN with the server"
	}
	if flagAuthStatusJSON {
		return printAuthStatusJSON(output)
	}
	ui.PrintInfo("Credential source: environment (XF_TOKEN)")
	if validationErr != nil {
		ui.PrintWarning("Token is present; server validity and expiry are unknown")
		return nil
	}
	if !result.Active {
		ui.PrintWarning("XF_TOKEN is inactive; replace it with a valid access token")
		return nil
	}
	ui.PrintSuccess("Authenticated (server validated)")
	if result.Exp > 0 {
		ui.PrintInfo("Expires: " + time.Unix(result.Exp, 0).Format(time.RFC1123))
	} else {
		ui.PrintInfo("Expiry: unknown")
	}
	return nil
}

func introspectAuthToken(ctx context.Context, token *auth.Token) (*auth.IntrospectResponse, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return auth.NewOAuthClient(&cfg.OAuth).IntrospectToken(ctx, token.AccessToken)
}
