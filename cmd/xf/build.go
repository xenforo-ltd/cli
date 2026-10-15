package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/build"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// buildCmd builds an add-on release from its build.json configuration.
var buildCmd = &cobra.Command{
	Use:   "build <addon-id>",
	Short: "Build an add-on release",
	Long: `Export an add-on's data, build JavaScript, run configured commands, and create
a release ZIP and hash manifest using build.json.

The XenForo installation is discovered from the current directory or XF_DIR.
Composer uses the Docker environment when configured, or the host otherwise.
Configured exec commands run on the host. Data is exported from prepared _output
after Composer and before JavaScript tasks; _metadata.jsonl drives each data type.
ZIP timestamps use the build start time; BUILD_DATE overrides them with
a Unix timestamp. Source modification times are not retained.`,
	Example: `  xf build Vendor/AddOn
  xf build Vendor/AddOn --skip-composer`,
	Args:    cobra.ExactArgs(1),
	GroupID: "run",
	RunE:    runBuild,
}

// buildJavaScriptCmd builds or watches an add-on's JavaScript without a release.
var buildJavaScriptCmd = &cobra.Command{
	Use:   "build-js <addon-id>",
	Short: "Build or watch an add-on's JavaScript",
	Long: `Run the minify and rollup tasks configured in the add-on's build.json.
Dependencies, exec commands, and release archives are not part of this command.

The XenForo installation is discovered from the current directory or XF_DIR.
Use --watch to rebuild when sources or configuration change. Build errors are
reported without stopping the watcher; press Ctrl-C to stop.`,
	Example: `  xf build-js Vendor/AddOn
  xf build-js Vendor/AddOn --watch`,
	Args:    cobra.ExactArgs(1),
	GroupID: "run",
	RunE:    runBuildJavaScript,
}

var (
	// flagBuildSkipComposer skips dependency installation for a release build.
	flagBuildSkipComposer bool
	// flagBuildWatch selects JavaScript watch mode.
	flagBuildWatch bool
)

// init registers the build commands and their flags.
func init() {
	buildCmd.Flags().BoolVar(&flagBuildSkipComposer, "skip-composer", false, "skip Composer dependency installation")
	buildJavaScriptCmd.Flags().BoolVar(&flagBuildWatch, "watch", false, "rebuild JavaScript when inputs change")
	rootCmd.AddCommand(buildCmd, buildJavaScriptCmd)
}

// runBuild forwards the release build to the build package and reports its outputs.
func runBuild(cmd *cobra.Command, args []string) error {
	xfDir, err := getXenForoDir(nil)
	if err != nil {
		return err
	}
	options := []build.Option{build.WithIO(os.Stdin, os.Stdout, os.Stderr)}
	if buildDate, set := os.LookupEnv("BUILD_DATE"); set {
		seconds, err := strconv.ParseInt(buildDate, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid BUILD_DATE: %w", err)
		}
		options = append(options, build.WithTimestamp(time.Unix(seconds, 0)))
	}
	if flagBuildSkipComposer {
		options = append(options, build.WithoutDependencyInstall())
	}
	result, err := build.Build(cmd.Context(), xfDir, args[0], options...)
	if err != nil {
		return err
	}
	ui.PrintSuccess("Built " + args[0])
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("Release", result.ZipPath),
		ui.KV("Hashes", result.HashesPath),
		ui.KV("Contents", ui.Plural(result.FileCount, "file", "files")+", "+ui.FormatBytes(result.TotalSize)),
	})
	return nil
}

// runBuildJavaScript forwards a build or watch operation to the build package.
func runBuildJavaScript(cmd *cobra.Command, args []string) error {
	xfDir, err := getXenForoDir(nil)
	if err != nil {
		return err
	}
	if flagBuildWatch {
		ui.PrintInfo("Watching JavaScript for " + args[0])
		return build.WatchJavaScript(cmd.Context(), xfDir, args[0], build.WatchOptions{
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		})
	}
	if err := build.BuildJavaScript(cmd.Context(), xfDir, args[0]); err != nil {
		return err
	}
	ui.PrintSuccess("Built JavaScript for " + args[0])
	return nil
}
