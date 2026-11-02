package main

import (
	"github.com/spf13/cobra"

	cachecommand "github.com/xenforo-ltd/cli/internal/commands/cache"
	"github.com/xenforo-ltd/cli/internal/completion"
)

var cacheCmd = &cobra.Command{
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
	// See the note on authCmd: NoArgs only takes effect when RunE is also set.
	Args:    cobra.NoArgs,
	GroupID: "maint",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var cacheListCmd = &cobra.Command{
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

var cachePurgeCmd = &cobra.Command{
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

var cachePathCmd = &cobra.Command{
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

var (
	flagCacheJSON       bool
	flagCacheLicenseKey string
	flagCacheAll        bool
)

func init() {
	cacheListCmd.Flags().BoolVar(&flagCacheJSON, "json", false, "output as JSON")
	cacheListCmd.Flags().StringVar(&flagCacheLicenseKey, "license", "", "filter by license key")

	cachePurgeCmd.Flags().StringVar(&flagCacheLicenseKey, "license", "", "only purge for specific license key")
	cachePurgeCmd.Flags().BoolVar(&flagCacheAll, "all", false, "confirm purging all cached downloads")

	completion.Register(cacheListCmd, "license", completion.CachedLicenseKeys)
	completion.Register(cachePurgeCmd, "license", completion.CachedLicenseKeys)

	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cachePurgeCmd)
	cacheCmd.AddCommand(cachePathCmd)

	rootCmd.AddCommand(cacheCmd)
}

func runCacheList(cmd *cobra.Command, args []string) error {
	return cachecommand.List(cmd.Context(), cachecommand.ListOptions{JSON: flagCacheJSON, LicenseKey: flagCacheLicenseKey})
}
func runCachePurge(cmd *cobra.Command, args []string) error {
	return cachecommand.Purge(cmd.Context(), cachecommand.PurgeOptions{LicenseKey: flagCacheLicenseKey, All: flagCacheAll})
}
func runCachePath(cmd *cobra.Command, args []string) error { return cachecommand.Path(cmd.Context()) }
