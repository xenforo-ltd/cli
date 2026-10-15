package build

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/xfcmd"
)

// composerTask installs production dependencies in the build root.
type composerTask struct {
	installationDir string
	dir             string
	streams         commandStreams
}

// run invokes Composer only if root contains composer.json, using the
// installation's environment when configured.
func (t composerTask) run(ctx context.Context, root *os.Root) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if _, err := root.Stat("composer.json"); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("reading composer.json: %w", err)
	}

	args := []string{"install", "--no-dev", "--no-interaction", "--no-progress"}
	if t.dir != "." {
		args = append(args, "--working-dir="+t.dir)
	}
	if err := xfcmd.RunComposer(ctx, t.installationDir, t.streams.stdin, t.streams.stdout, t.streams.stderr, args...); err != nil {
		return fmt.Errorf("composer install: %w", err)
	}

	return nil
}
