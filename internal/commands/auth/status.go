package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/ui"
)

type StatusOptions struct{ JSON bool }

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

func Status(ctx context.Context, opts StatusOptions) error {
	store, err := auth.NewStore()
	if err != nil {
		return reportAuthUnavailable(store, err, opts.JSON)
	}

	token, err := auth.RequireAuthFrom(store)
	if err != nil {
		return reportAuthUnavailable(store, err, opts.JSON)
	}

	if token.External {
		return runEnvironmentAuthStatus(ctx, token, opts.JSON)
	}

	expired := token.IsExpired()
	refreshable := token.RefreshToken != ""

	var (
		serverValid *bool
		username    string
	)

	if !expired {
		introspect, err := introspectAuthToken(ctx, token)
		if err == nil {
			serverValid = &introspect.Active
			username = introspect.Username
		}
	}

	if opts.JSON {
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

func reportAuthUnavailable(store auth.Store, err error, jsonOutput bool) error {
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
	if jsonOutput {
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

func runEnvironmentAuthStatus(ctx context.Context, token *auth.Token, jsonOutput bool) error {
	result, validationErr := introspectAuthToken(ctx, token)
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
	if jsonOutput {
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
