package db

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/passthrough"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// Shell opens the database client inside the container, forwarding any
// trailing arguments to it.
func Shell(ctx context.Context, args []string) error {
	xfDir, clientArgs, err := passthrough.Resolve(args)
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

	return openDatabaseShell(ctx, runner, info, clientArgs)
}

// openDatabaseShell runs the database client inside the database container,
// forwarding any extra arguments to it. The password travels in the
// environment rather than the argument list.
//
// The client needs the server in the running container, so a stopped
// environment is reported rather than falling back to a one-off container
// that has no server to connect to.
func openDatabaseShell(ctx context.Context, runner *environment.Runner, info connection, args []string) error {
	running, err := runner.IsServiceRunning(ctx, databaseService)
	if err != nil {
		return err
	}

	if !running {
		return clierror.WithHint(
			errors.New("the database is not running"),
			"Start the environment with "+ui.Command.Render("xf up"),
		)
	}

	env := map[string]string{"MYSQL_PWD": info.Password}

	cmd := databaseShellArgs(info, args)
	if err := runner.ExecOrRun(ctx, databaseService, env, os.Stdin, os.Stdout, os.Stderr, cmd...); err != nil {
		return clierror.Passthrough(err, "failed to open the database shell")
	}

	return nil
}

// databaseShellArgs builds the in-container client command. Extra arguments are
// appended verbatim, so callers can run a one-off query or pass any other
// client option.
func databaseShellArgs(info connection, extra []string) []string {
	args := []string{"mariadb", "--user=" + info.User, "--database=" + info.Name}

	return append(args, extra...)
}
