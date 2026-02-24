package auth

import (
	"os/exec"
	"runtime"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

func OpenBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return clierrors.Newf(clierrors.CodeInternal, "unsupported platform: %s", runtime.GOOS)
	}

	if err := cmd.Start(); err != nil {
		return clierrors.Wrap(clierrors.CodeInternal, "failed to open browser", err)
	}

	return nil
}
