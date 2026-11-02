package cli

import (
	"github.com/spf13/cobra"

	cachecommand "github.com/xenforo-ltd/cli/internal/commands/cache"
	"github.com/xenforo-ltd/cli/internal/completion"
)

var (
	// flagCacheJSON selects JSON cache list output.
	flagCacheJSON bool
	// flagCacheLicenseKey filters cache operations by license.
	flagCacheLicenseKey string
	// flagCacheAll confirms removal of all cached downloads.
	flagCacheAll bool
)

// newCacheCommand builds the cache command family and rejects unknown subcommands.
func newCacheCommand() *cobra.Command {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage cached downloads",
		Long: `Manage cached XenForo package downloads.

Downloaded packages are cached locally to avoid re-downloading. Use these
commands to view, manage, and clear the cache.`,
		Example: `  # List all cached downloads
  xf cache list

  # List cached downloads for a specific license
  xf cache list --license XF123-ABCD-1234

  # Show the cache directory path
  xf cache path

  # Clear all cached downloads
  xf cache purge --all`,
		Args:    cobra.NoArgs,
		GroupID: "maint",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cacheListCmd := &cobra.Command{
		Use:   "list",
		Short: "List cached downloads",
		Long: `Display all cached downloads with their metadata.

Shows download information including version, file size, checksum,
and download date. Results can be filtered by license key.`,
		Example: `  # List all cached downloads (compact table)
  xf cache list

  # List with full details
  xf cache list -v

  # List in JSON format
  xf cache list --json

  # List only downloads for a specific license
  xf cache list --license XF123-ABCD-1234`,
		Args: cobra.NoArgs,
		RunE: runCacheList,
	}

	cachePurgeCmd := &cobra.Command{
		Use:   "purge",
		Short: "Remove cached downloads",
		Long: `Remove cached downloads to free up disk space.

By default, requires explicit confirmation with --all flag. Use --license
to selectively remove downloads for a specific license.`,
		Example: `  # Remove all cached downloads
  xf cache purge --all

  # Remove downloads for a specific license only
  xf cache purge --license XF123-ABCD-1234`,
		Args: cobra.NoArgs,
		RunE: runCachePurge,
	}

	cachePathCmd := &cobra.Command{
		Use:   "path",
		Short: "Show cache directory location",
		Long: `Display the path to the cache directory.

Useful for scripting or manually inspecting cached files.`,
		Example: `  # Show cache path
  xf cache path

  # Open cache directory in file manager (macOS)
  open $(xf cache path)`,
		Args: cobra.NoArgs,
		RunE: runCachePath,
	}

	cacheListCmd.Flags().BoolVar(&flagCacheJSON, "json", false, "output as JSON")
	cacheListCmd.Flags().StringVar(&flagCacheLicenseKey, "license", "", "filter by license key")

	cachePurgeCmd.Flags().StringVar(&flagCacheLicenseKey, "license", "", "only purge for specific license key")
	cachePurgeCmd.Flags().BoolVar(&flagCacheAll, "all", false, "confirm purging all cached downloads")

	completion.Register(cacheListCmd, "license", completion.CachedLicenseKeys)
	completion.Register(cachePurgeCmd, "license", completion.CachedLicenseKeys)

	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cachePurgeCmd)
	cacheCmd.AddCommand(cachePathCmd)

	return cacheCmd
}

// runCacheList displays cached downloads with the selected filter and format.
func runCacheList(cmd *cobra.Command, args []string) error {
	return cachecommand.List(cmd.Context(), cachecommand.ListOptions{JSON: flagCacheJSON, LicenseKey: flagCacheLicenseKey})
}

// runCachePurge removes cached downloads according to the selected scope.
func runCachePurge(cmd *cobra.Command, args []string) error {
	return cachecommand.Purge(cmd.Context(), cachecommand.PurgeOptions{LicenseKey: flagCacheLicenseKey, All: flagCacheAll})
}

// runCachePath prints the cache directory.
func runCachePath(cmd *cobra.Command, args []string) error { return cachecommand.Path(cmd.Context()) }
