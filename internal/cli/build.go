package cli

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/build"
)

var (
	// flagBuildSkipComposer skips dependency installation for a release build.
	flagBuildSkipComposer bool
	// flagBuildWatch selects JavaScript watch mode.
	flagBuildWatch bool
)

// newBuildCommand builds the build command family.
func newBuildCommand() *cobra.Command {
	buildCmd := &cobra.Command{
		Use:   "build [addon-id]",
		Short: "Build an add-on release",
		Long: `Export an add-on's data, build JavaScript, run configured commands, and create
a release ZIP and hash manifest using build.json.

The XenForo installation is discovered from the current directory or XF_DIR.
When omitted, the add-on is inferred by searching upward for addon.json,
falling back to XF.
Composer uses the Docker environment when configured, or the host otherwise.
Exec commands run on the host unless they start with @php, @composer, or @xf,
which use the environment when configured and the host otherwise.
Data is exported from prepared _output after Composer and before JavaScript
tasks; _metadata.jsonl drives each data type.
ZIP timestamps use the build start time; BUILD_DATE overrides them with
a Unix timestamp. Source modification times are not retained.`,
		Example: `  xf build
  xf build Vendor/AddOn
  xf build Vendor/AddOn --skip-composer
  xf build js --watch`,
		Args:    cobra.MaximumNArgs(1),
		GroupID: "run",
		RunE:    runBuild,
	}

	buildJavaScriptCmd := &cobra.Command{
		Use:   "js [addon-id]",
		Short: "Build or watch an add-on's JavaScript",
		Long: `Run the minify and rollup tasks configured in the add-on's build.json.
Dependencies, exec commands, and release archives are not part of this command.

The XenForo installation is discovered from the current directory or XF_DIR.
When omitted, the add-on is inferred by searching upward for addon.json,
falling back to XF.
Use --watch to rebuild when sources or configuration change. Build errors are
reported without stopping the watcher; press Ctrl-C to stop.`,
		Example: `  xf build js
  xf build js --watch
  xf build js Vendor/AddOn
  xf build js Vendor/AddOn --watch`,
		Args: cobra.MaximumNArgs(1),
		RunE: runBuildJavaScript,
	}

	buildCmd.Flags().BoolVar(&flagBuildSkipComposer, "skip-composer", false, "skip Composer dependency installation")
	buildJavaScriptCmd.Flags().BoolVar(&flagBuildWatch, "watch", false, "rebuild JavaScript when inputs change")
	buildCmd.AddCommand(buildJavaScriptCmd)

	return buildCmd
}

// runBuild forwards the release build to the build package.
func runBuild(cmd *cobra.Command, args []string) error {
	return build.Run(cmd.Context(), args, flagBuildSkipComposer)
}

// runBuildJavaScript forwards a build or watch operation to the build package.
func runBuildJavaScript(cmd *cobra.Command, args []string) error {
	return build.JavaScript(cmd.Context(), args, flagBuildWatch)
}
