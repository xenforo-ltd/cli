package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"xf/internal/auth"
	"xf/internal/config"
	"xf/internal/errors"
	"xf/internal/ui"
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

	authStatusCmd.Flags().BoolVar(&flagAuthStatusJSON, "json", false, "output as JSON")
	authLoginCmd.Flags().IntVar(&flagAuthTimeout, "timeout", 300, "timeout in seconds for browser authentication")

	rootCmd.AddCommand(authCmd)
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return errors.New(errors.CodeKeychainUnavailable,
			"system keychain is not available - this is required for secure token storage")
	}

	pkce, err := auth.GeneratePKCE()
	if err != nil {
		return errors.Wrap(errors.CodeInternal, "failed to generate PKCE parameters", err)
	}

	oauthConfig := auth.DefaultOAuthConfig()
	client := auth.NewOAuthClient(oauthConfig)

	callbackServer, err := auth.NewCallbackServer(oauthConfig.RedirectPath)
	if err != nil {
		return err
	}
	callbackServer.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = callbackServer.Shutdown(ctx)
	}()

	redirectURI := callbackServer.RedirectURI()
	authURL := client.AuthorizationURL(pkce, redirectURI)

	ui.PrintInfo("Opening browser for authentication...")
	fmt.Printf("If the browser doesn't open, visit this URL:\n%s\n\n", ui.URL.Render(authURL))

	if err := auth.OpenBrowser(authURL); err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not open browser automatically: %v", err))
	}

	ui.PrintInfo("Waiting for authentication...")

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(flagAuthTimeout)*time.Second)
	defer cancel()

	result, err := callbackServer.WaitForCallback(ctx)
	if err != nil {
		return err
	}

	if result.Error != "" {
		return errors.Newf(errors.CodeAuthFailed, "authentication failed: %s", result.Error)
	}

	if result.State != pkce.State {
		return errors.New(errors.CodeAuthFailed, "authentication failed: state mismatch (possible CSRF attack)")
	}

	ui.PrintInfo("Exchanging authorization code for tokens...")

	token, err := client.ExchangeCode(ctx, result.Code, pkce, redirectURI)
	if err != nil {
		return err
	}

	if err := kc.SaveToken(token); err != nil {
		return err
	}

	ui.PrintSuccess("Authentication successful!")

	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		if flagAuthStatusJSON {
			data, err := json.Marshal(map[string]interface{}{
				"authenticated": false,
				"error":         "keychain unavailable",
			})
			if err != nil {
				return errors.Wrap(errors.CodeInternal, "failed to marshal auth status", err)
			}
			fmt.Println(string(data))
			return nil
		}
		ui.PrintWarning("Not authenticated (keychain unavailable)")
		return nil
	}

	token, err := kc.LoadToken()
	if err != nil {
		if errors.Is(err, errors.CodeAuthRequired) {
			if flagAuthStatusJSON {
				data, err := json.Marshal(map[string]interface{}{
					"authenticated": false,
				})
				if err != nil {
					return errors.Wrap(errors.CodeInternal, "failed to marshal auth status", err)
				}
				fmt.Println(string(data))
				return nil
			}
			ui.PrintWarning("Not authenticated")
			fmt.Printf("Run %s to authenticate.\n", ui.Command.Render("xf auth login"))
			return nil
		}
		return err
	}

	var serverValid *bool
	var username string

	if !token.IsExpired() {
		client := auth.NewOAuthClient(&auth.OAuthConfig{
			BaseURL: token.BaseURL,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		introspect, err := client.IntrospectToken(ctx, token.AccessToken)
		if err == nil {
			serverValid = &introspect.Active
			username = introspect.Username
		}
	}

	if flagAuthStatusJSON {
		output := map[string]interface{}{
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
			return errors.Wrap(errors.CodeInternal, "failed to marshal auth status", err)
		}
		fmt.Println(string(data))
		return nil
	}

	ui.PrintSuccess("Authenticated")
	fmt.Println()

	pairs := []ui.KVPair{}
	if username != "" {
		pairs = append(pairs, ui.KV("User", username))
	}
	pairs = append(pairs, ui.KV("Scope", token.Scope))
	pairs = append(pairs, ui.KV("Issued", token.IssuedAt.Format(time.RFC1123)))
	pairs = append(pairs, ui.KV("Expires", token.ExpiresAt.Format(time.RFC1123)))
	ui.PrintKeyValuePadded(pairs)

	fmt.Println()
	if token.IsExpired() {
		fmt.Printf("%s %s\n", ui.StatusIcon("error"), ui.Error.Render("Token EXPIRED"))
		if token.RefreshToken != "" {
			ui.PrintDetail("Token can be refreshed automatically")
		}
	} else {
		remaining := token.TimeUntilExpiry().Round(time.Minute)
		fmt.Printf("%s Token valid (%s remaining)\n", ui.StatusIcon("success"), ui.Success.Render(remaining.String()))
	}

	if serverValid != nil {
		if *serverValid {
			fmt.Printf("%s Server validation: %s\n", ui.StatusIcon("success"), ui.Success.Render("Active"))
		} else {
			fmt.Printf("%s Server validation: %s\n", ui.StatusIcon("error"), ui.Error.Render("Revoked or Invalid"))
		}
	}

	return nil
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return errors.New(errors.CodeKeychainUnavailable,
			"system keychain is not available")
	}

	token, err := kc.LoadToken()
	if err != nil {
		if errors.Is(err, errors.CodeAuthRequired) {
			ui.PrintInfo("Already logged out.")
			return nil
		}
		return err
	}

	client := auth.NewOAuthClient(&auth.OAuthConfig{
		BaseURL: token.BaseURL,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.RevokeToken(ctx, token.AccessToken); err != nil {
		if flagVerbose {
			ui.PrintWarning(fmt.Sprintf("Could not revoke token on server: %v", err))
		}
	}

	if token.RefreshToken != "" {
		if err := client.RevokeToken(ctx, token.RefreshToken); err != nil {
			if flagVerbose {
				ui.PrintWarning(fmt.Sprintf("Could not revoke refresh token on server: %v", err))
			}
		}
	}

	if err := kc.DeleteToken(); err != nil {
		return err
	}

	ui.PrintSuccess("Logged out successfully.")
	return nil
}

func runAuthRefresh(cmd *cobra.Command, args []string) error {
	kc := auth.NewKeychain()

	if !kc.IsAvailable() {
		return errors.New(errors.CodeKeychainUnavailable,
			"system keychain is not available")
	}

	token, err := kc.LoadToken()
	if err != nil {
		return err
	}

	if token.RefreshToken == "" {
		return errors.New(errors.CodeAuthFailed, "no refresh token available - run 'xf auth login'")
	}

	ui.PrintInfo("Refreshing access token...")

	client := auth.NewOAuthClient(&auth.OAuthConfig{
		BaseURL:  token.BaseURL,
		ClientID: config.GetEffectiveClientID(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	newToken, err := client.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return errors.Wrap(errors.CodeAuthFailed, "failed to refresh token", err)
	}

	if err := kc.SaveToken(newToken); err != nil {
		return err
	}

	ui.PrintSuccess("Token refreshed successfully!")
	fmt.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("New expiry", newToken.ExpiresAt.Format(time.RFC1123)),
		ui.KV("Time until expiry", newToken.TimeUntilExpiry().Round(time.Minute).String()),
	})

	return nil
}
