package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
)

type LoginOptions struct{ Timeout int }

func Login(commandCtx context.Context, opts LoginOptions) error {
	store, err := auth.NewWritableStore()
	if err != nil {
		return err
	}

	if err := store.PrepareLogin(); err != nil {
		return err
	}

	if file, ok := store.(*auth.FileStore); ok {
		ui.PrintInfo("Credential file: " + ui.Path.Render(file.Path()) + " (plaintext; keep out of version control)")
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

	callbackServer, err := auth.NewCallbackServer(commandCtx, cfg.OAuth.RedirectPath)
	if err != nil {
		return fmt.Errorf("failed to start OAuth callback server: %w", err)
	}

	callbackServer.Start()

	defer func() {
		ctx, cancel := context.WithTimeout(commandCtx, 5*time.Second)
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

	if err := auth.OpenBrowser(commandCtx, authURL); err != nil {
		// Stop the spinner before printing so the warning lands on its own
		// line instead of being overwritten by the next animation frame.
		// Restart only if it was animating: without a TTY, Stop is a no-op
		// and Start would reprint the message.
		animating := spinner.Animating()
		spinner.Stop()
		ui.PrintWarning("Could not open the browser automatically — use the URL above")
		if animating {
			spinner.Start()
		}
	}

	spinner.UpdateMessage("Waiting for authentication in the browser")

	ctx, cancel := context.WithTimeout(commandCtx, time.Duration(opts.Timeout)*time.Second)
	defer cancel()

	result, err := callbackServer.WaitForCallback(ctx)
	if err != nil {
		spinner.Stop()
		return fmt.Errorf("failed to wait for authentication callback: %w", err)
	}

	if result.Error != "" {
		spinner.Stop()
		return fmt.Errorf("authentication failed: %s: %w", result.Error, clierror.ErrAuthFailed)
	}

	if result.State != pkce.State {
		spinner.Stop()
		return fmt.Errorf("authentication failed: state mismatch (possible CSRF attack): %w", clierror.ErrAuthFailed)
	}

	spinner.UpdateMessage("Completing authentication")

	token, err := client.ExchangeCode(ctx, result.Code, pkce, redirectURI)
	if err != nil {
		spinner.Stop()
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	if err := store.SaveToken(token); err != nil {
		spinner.Stop()
		return fmt.Errorf("failed to save authentication token: %w", err)
	}

	message := "Authentication complete"

	ctx2, cancel2 := context.WithTimeout(commandCtx, 10*time.Second)
	defer cancel2()

	if introspect, err := client.IntrospectToken(ctx2, token.AccessToken); err == nil && introspect.Username != "" {
		message = "Authenticated as " + ui.Bold.Render(introspect.Username)
	}

	spinner.StopWithMessage("success", message)

	return nil
}
