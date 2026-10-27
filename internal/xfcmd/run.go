package xfcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
)

func RunInDir(ctx context.Context, installationDir, workDir string, stdin io.Reader, stdout, stderr io.Writer, argv ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(argv) == 0 {
		return errors.New("no command specified")
	}

	runner, err := dockercompose.NewRunner(installationDir)
	if err == nil {
		containerDir := ""
		if workDir != "." {
			containerDir = path.Join("/var/www/html", workDir)
		}
		return runner.ExecOrRunInDir(ctx, "xf", containerDir, nil, stdin, stdout, stderr, argv...)
	}
	if !errors.Is(err, dockercompose.ErrEnvNotInitialized) {
		return fmt.Errorf("initializing Docker Compose runner: %w", err)
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = filepath.Join(installationDir, filepath.FromSlash(workDir))
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
