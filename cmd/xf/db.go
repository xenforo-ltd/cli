package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/database"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var (
	flagDBPrintURL bool
	flagDBShell    bool
)

// databaseService is the compose service that runs the MySQL server.
const databaseService = "mysql"

var dbCmd = &cobra.Command{
	Use:     "db [path]",
	Aliases: []string{"database"},
	Short:   "Open the environment's database",
	Long: `Open the database for a XenForo environment.

On macOS with OrbStack the database is opened in TablePlus, using the
container's orb.local hostname, so no ports have to be published.

Currently only MySQL environments are supported.`,
	Example: `  # Open the database in TablePlus
  xf db

  # Print the connection URL instead of opening a client
  xf db --print-url

  # Open the database client inside the container (any platform)
  xf db --shell`,
	Args:    cobra.MaximumNArgs(1),
	GroupID: "env",
	RunE:    runDB,
}

func init() {
	dbCmd.Flags().BoolVar(&flagDBPrintURL, "print-url", false, "print the connection URL and exit")
	dbCmd.Flags().BoolVar(&flagDBShell, "shell", false, "open the database client inside the container")
	rootCmd.AddCommand(dbCmd)
}

func runDB(cmd *cobra.Command, args []string) error {
	if flagDBPrintURL && flagDBShell {
		return newUsageError(errors.New("--print-url and --shell cannot be used together"))
	}

	xfDir, err := getXenForoDir(args)
	if err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	info, err := resolveDatabaseInfo(runner)
	if err != nil {
		return err
	}

	if flagDBPrintURL {
		if err := requireOrbStack(cmd.Context()); err != nil {
			return err
		}

		// The URL carries the local development password so it can be piped
		// straight into a database client.
		// codeql[go/clear-text-logging] --print-url is an explicit request to expose it.
		fmt.Println(info.URL())
		return nil
	}

	if flagDBShell {
		return openDatabaseShell(cmd.Context(), runner, info)
	}

	return openInTablePlus(cmd.Context(), info)
}

// resolveDatabaseInfo turns the environment's contexts and credentials into a
// connection description. Only MySQL is supported for now; the driver is
// detected so the unsupported case can say which engine it found.
func resolveDatabaseInfo(runner *dockercompose.Runner) (database.Info, error) {
	driver, err := database.Detect(runner.Contexts())
	if err != nil {
		return database.Info{}, withHint(err, "Set XF_CONTEXTS to include a database, for example "+ui.Command.Render("mysql"))
	}

	if driver != database.DriverMySQL {
		return database.Info{}, withHint(
			fmt.Errorf("%s databases are not supported by %s yet", driver, ui.Command.Render("xf db")),
			"Only MySQL environments are supported for now",
		)
	}

	user, password := runner.DatabaseCredentials()

	return database.Info{
		Driver:   driver,
		Host:     database.OrbStackHost(databaseService, runner.Instance()),
		Port:     driver.DefaultPort(),
		User:     user,
		Password: password,
		Name:     runner.DatabaseName(),
	}, nil
}

// openDatabaseShell runs the database client inside the database container.
// The password travels in the environment rather than the argument list.
func openDatabaseShell(ctx context.Context, runner *dockercompose.Runner, info database.Info) error {
	env := map[string]string{"MYSQL_PWD": info.Password}

	err := runner.ExecOrRun(ctx, databaseService, env, os.Stdin, os.Stdout, os.Stderr, "mariadb", "--user="+info.User, info.Name)
	if err != nil {
		return passthroughError(err, "failed to open the database shell")
	}

	return nil
}

// requireOrbStack ensures the Docker engine is OrbStack, which resolves
// container hostnames as <service>.<instance>.orb.local from the host without
// publishing a port.
func requireOrbStack(ctx context.Context) error {
	isOrbStack, err := dockercompose.IsOrbStack(ctx)
	if err != nil {
		return fmt.Errorf("failed to detect OrbStack: %w", err)
	}

	if !isOrbStack {
		return withHint(
			errors.New("this command requires OrbStack"),
			"The orb.local hostname cannot be resolved without OrbStack; use "+ui.Command.Render("xf db --shell")+" instead",
		)
	}

	return nil
}

// openInTablePlus launches TablePlus with the connection URL. It relies on
// OrbStack, which makes the container's orb.local hostname resolvable from
// macOS without publishing a port.
func openInTablePlus(ctx context.Context, info database.Info) error {
	if runtime.GOOS != "darwin" {
		return withHint(
			errors.New("opening a database client is currently supported on macOS only"),
			"Use "+ui.Command.Render("xf db --shell")+" instead",
		)
	}

	if err := requireOrbStack(ctx); err != nil {
		return err
	}

	if _, err := exec.LookPath("open"); err != nil {
		return fmt.Errorf("failed to find the macOS open command: %w", err)
	}

	if err := exec.CommandContext(ctx, "open", "-a", "TablePlus", info.URL()).Run(); err != nil {
		return withHint(
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
