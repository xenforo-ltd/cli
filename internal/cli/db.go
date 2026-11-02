package cli

import (
	"github.com/spf13/cobra"

	dbcommand "github.com/xenforo-ltd/cli/internal/commands/db"
)

var (
	flagDBPrintURL bool
)

// newDBCommand builds the db command family.
func newDBCommand() *cobra.Command {
	dbCmd := &cobra.Command{
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
  xf db shell`,
		Args:    cobra.MaximumNArgs(1),
		GroupID: "env",
		RunE:    runDB,
	}

	dbShellCmd := &cobra.Command{
		Use:   "shell [path] [args...]",
		Short: "Open the database client inside the container",
		Long: `Open the database client inside the database container.

If no path is provided, the current directory will be searched for a XenForo
installation.

Everything after 'shell' is passed to the client, including flags, so queries
and client options work as they would locally. Give xf's own flags before the
command name, and use 'xf help db shell' for this help.`,
		Example: `  # Open an interactive session
  xf db shell

  # Run a query and exit
  xf db shell -e 'show tables;'`,
		// Everything after this command belongs to the wrapped client, including
		// flags. xf's own flags must be given before the command name.
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE:               runDBShell,
	}

	dbCmd.Flags().BoolVar(&flagDBPrintURL, "print-url", false, "print the connection URL and exit")
	dbCmd.AddCommand(dbShellCmd)

	return dbCmd
}

func runDB(cmd *cobra.Command, args []string) error {
	return dbcommand.DB(cmd.Context(), args, dbcommand.Options{PrintURL: flagDBPrintURL})
}
func runDBShell(cmd *cobra.Command, args []string) error { return dbcommand.Shell(cmd.Context(), args) }
