package composer

import (
	"context"
	"os"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
)

// Run forwards arguments and standard streams to Composer in the environment.
func Run(ctx context.Context, args []string) error {
	xfDir, composerArgs, err := passthrough.Resolve(args)
	if err != nil {
		return err
	}

	if err := environment.RunComposer(ctx, xfDir, os.Stdin, os.Stdout, os.Stderr, composerArgs...); err != nil {
		return clierror.Passthrough(err, "failed to run Composer")
	}

	return nil
}
