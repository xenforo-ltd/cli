package build

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// commandStreams holds the standard streams for subprocesses.
type commandStreams struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// runCommand runs name with args and streams in dir, relative to root.
// The subprocess is not sandboxed by root.
func runCommand(ctx context.Context, root *os.Root, streams commandStreams, dir, name string, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	working, err := root.OpenRoot(filepath.FromSlash(dir))
	if err != nil {
		return err
	}

	defer working.Close()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = working.Name()
	cmd.Stdin = streams.stdin
	cmd.Stdout = streams.stdout
	cmd.Stderr = streams.stderr
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	return err
}
