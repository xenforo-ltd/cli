package auth

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

// OpenBrowser opens a URL in the user's default browser.
func OpenBrowser(ctx context.Context, urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL %s: %w", urlStr, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme %s: %w", u.Scheme, ErrInvalidInput)
	}

	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", u.String())
	case "linux":
		cmd = exec.CommandContext(ctx, "xdg-open", u.String())
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", u.String())
	default:
		return fmt.Errorf("unsupported platform %s: %w", runtime.GOOS, ErrUnsupported)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	return nil
}
