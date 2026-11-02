package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/update"
	"github.com/xenforo-ltd/cli/internal/passthrough"
)

var (
	// flagUpdateSkipPull skips pulling base images.
	flagUpdateSkipPull bool
	// flagUpdateSkipBuild skips building images.
	flagUpdateSkipBuild bool
	// flagUpdateSkipRestart leaves running containers unchanged.
	flagUpdateSkipRestart bool
)

// newUpdateCommand builds the update command.
func newUpdateCommand() *cobra.Command {
	updateCmd := &cobra.Command{
		Use:   "update [path]",
		Short: "Update the Docker environment from the CLI",
		Long: `Refresh a XenForo project's Docker environment from the files shipped with
this version of the CLI.

Environment files are rewritten, except .env and .dockerignore: those are
only created when missing, and a copy of the shipped default is written
alongside as .env.default or .dockerignore.default when the existing file
differs. Images are pulled and rebuilt, and running containers are updated
to the new images.

Running it repeatedly is safe: files that already match are left alone and
compose reports nothing to do.`,
		Example: `  # Update the environment in the current directory
  xf update

  # Update a specific project
  xf update ./my-project

  # Rebuild with the cached base images
  xf update --no-pull

  # Only refresh files, leaving containers untouched
  xf update --no-restart`,
		Args:    cobra.MaximumNArgs(1),
		GroupID: "env",
		RunE:    runUpdate,
	}

	updateCmd.Flags().BoolVar(&flagUpdateSkipPull, "no-pull", false, "skip pulling images")
	updateCmd.Flags().BoolVar(&flagUpdateSkipBuild, "no-build", false, "skip building images")
	updateCmd.Flags().BoolVar(&flagUpdateSkipRestart, "no-restart", false, "leave running containers on their current images")

	return updateCmd
}

// runUpdate refreshes environment files and applies the selected image options.
func runUpdate(cmd *cobra.Command, args []string) error {
	dir, err := passthrough.XenForoDir(args)
	if err != nil {
		return err
	}

	return update.Run(cmd.Context(), update.Options{
		Path:        dir,
		SkipPull:    flagUpdateSkipPull,
		SkipBuild:   flagUpdateSkipBuild,
		SkipRestart: flagUpdateSkipRestart,
	})
}
