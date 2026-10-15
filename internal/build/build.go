// Package build creates XenForo add-on releases and builds JavaScript assets.
package build

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"
)

// BuildResult describes a completed release.
type BuildResult struct {
	// HashesPath is the absolute path to the generated hash manifest.
	HashesPath string

	// ZipPath is the absolute path to the release archive.
	ZipPath string

	// FileCount counts archived files other than the generated hash manifest.
	FileCount int

	// TotalSize is the total uncompressed size of the files counted by FileCount,
	// in bytes.
	TotalSize int64
}

// Option configures a call to [Build].
type Option func(*buildOptions)

// buildOptions holds release settings and subprocess streams.
type buildOptions struct {
	skipDependencyInstall bool
	timestamp             time.Time
	commandStreams
}

// WithoutDependencyInstall skips Composer installation without skipping other
// build tasks.
func WithoutDependencyInstall() Option {
	return func(o *buildOptions) {
		o.skipDependencyInstall = true
	}
}

// WithIO sets the streams for subprocesses and build warnings.
//
// A nil stdin provides no input; nil stdout and stderr discard output.
// These are also the defaults when WithIO is not supplied.
func WithIO(stdin io.Reader, stdout, stderr io.Writer) Option {
	return func(o *buildOptions) {
		o.stdin, o.stdout, o.stderr = stdin, stdout, stderr
	}
}

// WithTimestamp sets the modification time of every ZIP entry, including
// generated files. The time is normalized to UTC with whole-second precision
// and must fall between 1980-01-01 and 2106-02-07 06:28:15 UTC.
// Without this option, Build uses 1980-01-01 00:00:00 UTC.
func WithTimestamp(timestamp time.Time) Option {
	return func(o *buildOptions) {
		o.timestamp = timestamp
	}
}

// Build runs an add-on's build tasks and writes its hash manifest and release ZIP.
//
// rootDir names the XenForo installation; addOnID is an ID such as "Vendor/AddOn".
// The add-on's optional build.json determines the build root and file selection.
// Without it, all eligible files beneath the add-on directory are included.
// Composer uses the project's Docker environment when configured and runs on
// the host otherwise. Configured exec commands always run on the host.
// Data is exported from prepared _output after Composer and before JavaScript
// tasks. The export creates the add-on's _data directory if needed.
// ZIP timestamps are fixed by default and do not use source modification times.
func Build(ctx context.Context, rootDir, addOnID string, options ...Option) (*BuildResult, error) {
	opts := applyOptions(options)
	if opts.timestamp.Before(time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)) ||
		opts.timestamp.After(time.Unix(1<<32-1, 0)) {
		return nil, errors.New("archive timestamp must be between 1980-01-01 and 2106-02-07 06:28:15 UTC")
	}

	installation, err := openInstallation(ctx, rootDir)
	if err != nil {
		return nil, fmt.Errorf("opening XenForo root: %w", err)
	}

	defer installation.Close()
	cfg, root, err := loadBuildRoot(ctx, installation, addOnID)
	if err != nil {
		return nil, err
	}

	defer root.Close()

	if cfg.includeAll {
		if _, err := fmt.Fprintln(opts.stderr, "warning: no whitelist configured; including every eligible file in the build root"); err != nil {
			return nil, fmt.Errorf("writing build warning: %w", err)
		}
	}

	selection := uploadSelection{
		addOnPaths:       cfg.addOnPaths,
		whitelist:        cfg.whitelist,
		blacklist:        cfg.blacklist,
		additional:       cfg.additional,
		composerAutoload: cfg.composerAutoload,
	}

	hashesPath := path.Join(cfg.addOnDir, "hashes.json")
	zipPath := path.Join(cfg.addOnDir, "_releases", cfg.releaseName)
	var tasks []task
	if !opts.skipDependencyInstall {
		tasks = append(tasks, composerTask{installationDir: installation.Name(), dir: cfg.prefix, streams: opts.commandStreams})
	}

	tasks = append(tasks, exportDataTask{dir: cfg.addOnDir, stderr: opts.stderr})
	tasks = append(tasks, javascriptTasks(cfg)...)
	if len(cfg.commands) > 0 {
		tasks = append(tasks, execTask{commands: cfg.commands, dir: cfg.addOnDir, streams: opts.commandStreams})
	}

	hashes := &hashesTask{selection: selection, destination: hashesPath, exclude: cfg.hashExclude}
	tasks = append(tasks, hashes)
	if err := runTasks(ctx, root, tasks); err != nil {
		return nil, err
	}

	archive := &zipTask{selection: selection, upload: hashes.upload, hashesPath: hashesPath, zipPath: zipPath, timestamp: opts.timestamp}
	if err := runTasks(ctx, root, []task{archive}); err != nil {
		return nil, err
	}

	return &BuildResult{
		HashesPath: filepath.Join(root.Name(), filepath.FromSlash(hashesPath)),
		ZipPath:    filepath.Join(root.Name(), filepath.FromSlash(zipPath)),
		FileCount:  archive.fileCount,
		TotalSize:  archive.totalSize,
	}, nil
}

// applyOptions applies options in order and supplies default output streams.
func applyOptions(options []Option) buildOptions {
	opts := buildOptions{timestamp: time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)}
	for _, option := range options {
		option(&opts)
	}

	opts.stdout = writerOrDiscard(opts.stdout)
	opts.stderr = writerOrDiscard(opts.stderr)
	opts.timestamp = opts.timestamp.UTC().Truncate(time.Second)

	return opts
}

// writerOrDiscard returns writer, or [io.Discard] if writer is nil.
func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}

	return writer
}

// task is a build stage that discovers its own inputs within the build root.
type task interface {
	run(context.Context, *os.Root) error
}

// runTasks runs tasks in order, stopping at the first error.
func runTasks(ctx context.Context, root *os.Root, tasks []task) error {
	for _, task := range tasks {
		if err := task.run(ctx, root); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("build interrupted: %w", err)
	}

	return nil
}
