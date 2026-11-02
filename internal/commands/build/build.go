package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/xenforo-ltd/cli/internal/build"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// Run forwards the release build to the build package and reports its outputs.
func Run(ctx context.Context, args []string, skipComposer bool) error {
	xfDir, addOnID, err := getBuildTarget(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	progress := &buildProgress{ctx: ctx, verbose: cfg.Verbose}
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

	if skipComposer {
		options = append(options, build.WithoutDependencyInstall())
	}

	result, err := build.Build(ctx, xfDir, addOnID, options...)
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
		ui.KV("Release", ui.FileLink(releaseDisplayPath(result.ZipPath), result.ZipPath)),
		ui.KV("Contents", fmt.Sprintf("%s, %s (%s uncompressed)",
			ui.Plural(result.FileCount, "file", "files"), ui.FormatBytes(archiveInfo.Size()), ui.FormatBytes(result.TotalSize))),
	})

	return nil
}

// releaseDisplayPath shows an archive relative to the current directory only
// when it is beneath that directory. Otherwise it keeps the absolute path.
func releaseDisplayPath(filename string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filename
	}

	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return filename
	}

	rel, err := filepath.Rel(cwd, filename)
	if err != nil || !filepath.IsLocal(rel) {
		return filename
	}

	return rel
}
