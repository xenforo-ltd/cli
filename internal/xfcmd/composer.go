package xfcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
)

// RunComposer runs Composer in the XenForo environment at xfDir, using exec for
// a running service or run for a stopped service. If no environment is configured,
// it runs Composer on the host in xfDir. Environment failures do not fall back
// to the host. Output writers must be non-nil.
func RunComposer(ctx context.Context, xfDir string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err == nil {
		return runner.ExecOrRun(ctx, "xf", nil, stdin, stdout, stderr, append([]string{"composer"}, args...)...)
	}
	if !errors.Is(err, dockercompose.ErrEnvNotInitialized) {
		return fmt.Errorf("initializing Docker Compose runner: %w", err)
	}

	cmd := exec.CommandContext(ctx, "composer", args...)
	cmd.Dir = xfDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}
