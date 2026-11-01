package main

import (
	"time"

	"github.com/spf13/cobra"

	authcommand "github.com/xenforo-ltd/cli/internal/commands/auth"
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
	return authcommand.Login(cmd.Context(), authcommand.LoginOptions{Timeout: flagAuthTimeout})
}
func runAuthStatus(cmd *cobra.Command, args []string) error {
	return authcommand.Status(cmd.Context(), authcommand.StatusOptions{JSON: flagAuthStatusJSON})
}
func runAuthLogout(cmd *cobra.Command, args []string) error { return authcommand.Logout(cmd.Context()) }
func runAuthRefresh(cmd *cobra.Command, args []string) error {
	return authcommand.Refresh(cmd.Context())
}
