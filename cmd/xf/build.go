package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/build"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// buildCmd builds an add-on release from its build.json configuration.
var buildCmd = &cobra.Command{
	Use:   "build [addon-id]",
	Short: "Build an add-on release",
	Long: `Export an add-on's data, build JavaScript, run configured commands, and create
a release ZIP and hash manifest using build.json.

The XenForo installation is discovered from the current directory or XF_DIR.
When omitted, the add-on is inferred by searching upward for addon.json,
falling back to XF.
Composer uses the Docker environment when configured, or the host otherwise.
Configured exec commands run on the host. Data is exported from prepared _output
after Composer and before JavaScript tasks; _metadata.jsonl drives each data type.
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

// buildJavaScriptCmd builds or watches an add-on's JavaScript without a release.
var buildJavaScriptCmd = &cobra.Command{
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
	buildCmd.AddCommand(buildJavaScriptCmd)
	rootCmd.AddCommand(buildCmd)
}

// runBuild forwards the release build to the build package and reports its outputs.
func runBuild(cmd *cobra.Command, args []string) error {
	xfDir, addOnID, err := getBuildTarget(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	progress := &buildProgress{ctx: cmd.Context(), verbose: cfg.Verbose}
	defer progress.stop()

	options := []build.Option{
		build.WithIO(os.Stdin, buildProgressWriter{progress, os.Stdout}, buildProgressWriter{progress, os.Stderr}),
		build.WithTaskProgress(progress.start),
	}
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
	result, err := build.Build(cmd.Context(), xfDir, addOnID, options...)
	if err != nil {
		return err
	}
	archiveInfo, err := os.Stat(result.ZipPath)
	if err != nil {
		return fmt.Errorf("reading release archive: %w", err)
	}
	ui.PrintSuccess("Built " + addOnID)
	ui.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("Release", ui.FileLink(result.ZipPath, result.ZipPath)),
		ui.KV("Contents", fmt.Sprintf("%s (%s uncompressed, %s)",
			ui.FormatBytes(archiveInfo.Size()), ui.FormatBytes(result.TotalSize), ui.Plural(result.FileCount, "file", "files"))),
	})
	return nil
}

// runBuildJavaScript forwards a build or watch operation to the build package.
func runBuildJavaScript(cmd *cobra.Command, args []string) error {
	xfDir, addOnID, err := getBuildTarget(args)
	if err != nil {
		return err
	}
	spinner := ui.NewSpinner("Building JavaScript")
	defer spinner.Stop()
	if flagBuildWatch {
		ui.PrintInfo("Watching JavaScript for " + addOnID)
		ui.Println()
		spinner.UpdateMessage("Rebuilding JavaScript")
		return build.WatchJavaScript(cmd.Context(), xfDir, addOnID, build.WatchOptions{
			Stderr: os.Stderr,
			OnBuild: func() func(error) {
				spinner.Start()
				return func(err error) {
					if err != nil {
						spinner.Stop()
					} else {
						spinner.StopWithMessage("success", "JavaScript rebuilt")
					}
				}
			},
		})
	}
	spinner.Start()
	if err := build.BuildJavaScript(cmd.Context(), xfDir, addOnID); err != nil {
		return err
	}
	spinner.StopWithMessage("success", "Built JavaScript for "+addOnID)
	return nil
}

// getBuildTarget resolves the installation and an explicit or enclosing add-on.
// Without an enclosing addon.json under src/addons, the target defaults to XF.
func getBuildTarget(args []string) (string, string, error) {
	xfDir, err := getXenForoDir(nil)
	if err != nil {
		return "", "", err
	}
	xfDir, err = filepath.Abs(xfDir)
	if err != nil {
		return "", "", fmt.Errorf("resolving XenForo root: %w", err)
	}
	xfDir, err = filepath.EvalSymlinks(xfDir)
	if err != nil {
		return "", "", fmt.Errorf("resolving XenForo root: %w", err)
	}
	if len(args) > 0 {
		return xfDir, args[0], nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("reading current directory: %w", err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", "", fmt.Errorf("resolving current directory: %w", err)
	}
	addOnsDir := filepath.Join(xfDir, "src", "addons")
	for {
		rel, err := filepath.Rel(addOnsDir, dir)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			break
		}
		metadataPath := filepath.Join(dir, "addon.json")
		info, err := os.Stat(metadataPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("reading %s: %w", metadataPath, err)
		}
		addOnID := filepath.ToSlash(rel)
		if err == nil && info.Mode().IsRegular() && strings.Count(addOnID, "/") <= 1 {
			return xfDir, addOnID, nil
		}
		dir = filepath.Dir(dir)
	}
	return xfDir, "XF", nil
}
