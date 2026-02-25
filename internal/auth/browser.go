// Package auth handles authentication and OAuth flows.
package auth

import (
	"context"
	"os/exec"
	"runtime"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

// OpenBrowser opens a URL in the user's default browser.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(context.Background(), "open", url)
	case "linux":
		cmd = exec.CommandContext(context.Background(), "xdg-open", url)
	case "windows":
		cmd = exec.CommandContext(context.Background(), "rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return clierrors.Newf(clierrors.CodeInternal, "unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return clierrors.Wrap(clierrors.CodeInternal, "failed to open browser", err)
	}

	return nil
}
