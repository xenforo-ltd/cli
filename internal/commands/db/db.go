package db

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
	"github.com/xenforo-ltd/cli/internal/ui"
)

type Options struct{ PrintURL bool }

func DB(ctx context.Context, args []string, opts Options) error {
	xfDir, err := passthrough.XenForoDir(args)
	if err != nil {
		return err
	}

	runner, err := environment.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	info, err := resolveDatabaseInfo(runner)
	if err != nil {
		return err
	}

	if opts.PrintURL {
		if err := requireOrbStack(ctx); err != nil {
			return err
		}

		// The URL carries the local development password so it can be piped
		// straight into a database client.
		// codeql[go/clear-text-logging] --print-url is an explicit request to expose it.
		fmt.Println(info.urlString())
		return nil
	}

	return openInTablePlus(ctx, info)
}

// requireOrbStack ensures the Docker engine is OrbStack, which resolves
// container hostnames as <service>.<instance>.orb.local from the host without
// publishing a port.
func requireOrbStack(ctx context.Context) error {
	isOrbStack, err := environment.IsOrbStack(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect OrbStack: %w", err)
	}

	if !isOrbStack {
		return clierror.WithHint(
			errors.New("this command requires OrbStack"),
			"The orb.local hostname cannot be resolved without OrbStack; use "+ui.Command.Render("xf db shell")+" instead",
		)
	}

	return nil
}

// openInTablePlus launches TablePlus with the connection URL. It relies on
// OrbStack, which makes the container's orb.local hostname resolvable from
// macOS without publishing a port.
func openInTablePlus(ctx context.Context, info info) error {
	if runtime.GOOS != "darwin" {
		return clierror.WithHint(
			errors.New("opening a database client is currently supported on macOS only"),
			"Use "+ui.Command.Render("xf db shell")+" instead",
		)
	}

	if err := requireOrbStack(ctx); err != nil {
		return err
	}

	if _, err := exec.LookPath("open"); err != nil {
		return fmt.Errorf("failed to find the macOS open command: %w", err)
	}

	if err := exec.CommandContext(ctx, "open", "-a", "TablePlus", info.urlString()).Run(); err != nil {
		return clierror.WithHint(
			fmt.Errorf("failed to open TablePlus: %w", err),
			"Install TablePlus from https://tableplus.com",
		)
	}

	ui.SuccessBox("Opened database in TablePlus", []ui.KVPair{
		ui.KV("Database", info.Name),
		ui.KV("Host", info.Host),
	})

	return nil
}
