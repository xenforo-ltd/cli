package auth

import (
	"context"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

// OpenBrowser opens a URL in the user's default browser.
func OpenBrowser(ctx context.Context, urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return clierrors.Newf(clierrors.CodeInternal, "invalid URL: %s", urlStr)
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
		return clierrors.Newf(clierrors.CodeInternal, "unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return clierrors.Wrap(clierrors.CodeInternal, "failed to open browser", err)
	}

	return nil
}
