package build

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/evanw/esbuild/pkg/api"
)

// WatchOptions controls reporting from [WatchJavaScript].
// Its zero value discards all output.
type WatchOptions struct {
	// Stdout receives successful build status messages.
	Stdout io.Writer

	// Stderr receives build errors.
	Stderr io.Writer
}

// WatchJavaScript rebuilds the add-on's JavaScript when sources or configuration
// change.
//
// rootDir and addOnID have the same meaning as in [Build].
// Build errors are reported on options.Stderr without stopping the watcher.
// It blocks until ctx is canceled or watching fails. Cancellation returns an
// error wrapping ctx.Err().
func WatchJavaScript(ctx context.Context, rootDir, addOnID string, options WatchOptions) error {
	if !addOnIDPattern.MatchString(addOnID) {
		return fmt.Errorf("invalid add-on ID: %q", addOnID)
	}

	stdout := writerOrDiscard(options.Stdout)
	stderr := writerOrDiscard(options.Stderr)
	installation, err := openInstallation(ctx, rootDir)
	if err != nil {
		return fmt.Errorf("opening XenForo root: %w", err)
	}

	defer installation.Close()
	failures := make(chan error, 1)
	watch, contextErr := api.Context(api.BuildOptions{
		AbsWorkingDir: installation.Name(),
		EntryPoints:   []string{"xf-javascript"},
		Bundle:        true,
		Write:         false,
		LogLevel:      api.LogLevelSilent,
		Plugins: []api.Plugin{{
			Name: "xf-javascript",
			Setup: func(build api.PluginBuild) {
				build.OnResolve(api.OnResolveOptions{Filter: "^xf-javascript$"}, func(api.OnResolveArgs) (api.OnResolveResult, error) {
					return api.OnResolveResult{Path: "xf-javascript", Namespace: "xf-javascript"}, nil
				})
				build.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "xf-javascript"}, func(api.OnLoadArgs) (api.OnLoadResult, error) {
					return loadJavaScriptWatch(ctx, installation, addOnID), nil
				})
				build.OnEnd(func(result *api.BuildResult) (api.OnEndResult, error) {
					if ctx.Err() != nil {
						return api.OnEndResult{}, nil
					}

					var err error
					if len(result.Errors) > 0 {
						_, err = fmt.Fprintln(stderr, "JavaScript build failed:", result.Errors[0].Text)
					} else {
						_, err = fmt.Fprintln(stdout, "JavaScript build complete")
					}

					if err != nil {
						select {
						case failures <- fmt.Errorf("writing JavaScript watch output: %w", err):
						default:
						}
					}

					return api.OnEndResult{}, nil
				})
			},
		}},
	})
	if contextErr != nil {
		return fmt.Errorf("creating JavaScript watch: %w", contextErr)
	}

	defer watch.Dispose()
	if err := watch.Watch(api.WatchOptions{}); err != nil {
		return fmt.Errorf("starting JavaScript watch: %w", err)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("watching JavaScript: %w", ctx.Err())
	case err := <-failures:
		return err
	}
}

// loadJavaScriptWatch reloads configuration, builds JavaScript, and records paths
// to watch. It records configuration files and their ancestor directories even
// when loading fails, allowing a later edit to recover.
func loadJavaScriptWatch(ctx context.Context, installation *os.Root, addOnID string) api.OnLoadResult {
	empty := ""
	result := api.OnLoadResult{Contents: &empty, Loader: api.LoaderJS}
	addOnDir := path.Join("src", "addons", addOnID)
	for _, name := range []string{"build.json", "addon.json"} {
		name = path.Join(addOnDir, name)
		if info, err := installation.Stat(filepath.FromSlash(name)); err == nil && info.Mode().IsRegular() {
			result.WatchFiles = append(result.WatchFiles, filepath.Join(installation.Name(), filepath.FromSlash(name)))
		}
	}

	for dir := addOnDir; ; dir = path.Dir(dir) {
		if info, err := installation.Stat(filepath.FromSlash(dir)); err == nil && info.IsDir() {
			result.WatchDirs = append(result.WatchDirs, filepath.Join(installation.Name(), filepath.FromSlash(dir)))
		}

		if dir == "." {
			break
		}
	}

	cfg, root, err := loadBuildRoot(ctx, installation, addOnID)
	if err != nil {
		result.Errors = []api.Message{{Text: err.Error()}}
		return result
	}

	defer root.Close()
	buildErr := runTasks(ctx, root, javascriptTasks(cfg))
	files, dirs, err := javascriptWatchPaths(ctx, root, cfg)
	result.WatchFiles = append(result.WatchFiles, files...)
	result.WatchDirs = append(result.WatchDirs, dirs...)
	if buildErr != nil {
		result.Errors = []api.Message{{Text: buildErr.Error()}}
	} else if err != nil {
		result.Errors = []api.Message{{Text: err.Error()}}
	}

	return result
}

// javascriptWatchPaths returns input files and directories whose entries can
// affect the configured tasks. It retains matching paths when selection fails
// so the watcher can detect recovery.
func javascriptWatchPaths(ctx context.Context, root *os.Root, cfg *config) ([]string, []string, error) {
	patterns := slices.Clone(cfg.minify)
	for _, entry := range cfg.rollups {
		patterns = append(patterns, entry.inputs...)
	}
	files, err := cfg.collectJavaScriptFiles(ctx, root, patterns)
	if err != nil {
		return nil, nil, fmt.Errorf("collecting JavaScript watch files: %w", err)
	}

	watchFiles := make(map[string]bool)
	watchDirs := make(map[string]bool)
	var selectionErr error
	recordSelection := func(matches []file, err error) []file {
		if err != nil && selectionErr == nil {
			selectionErr = err
		}

		return matches
	}

	outputs := rollupOutputs(cfg.rollups)
	for _, pattern := range cfg.minify {
		for _, f := range recordSelection(cfg.minifyFiles(ctx, files, pattern, outputs)) {
			if !cfg.declaredOutput(f.path, outputs) {
				watchFiles[filepath.Join(root.Name(), filepath.FromSlash(f.path))] = true
			}
		}
	}

	for i, entry := range cfg.rollups {
		for _, pattern := range entry.inputs {
			for _, f := range recordSelection(cfg.rollupFiles(ctx, files, pattern, outputs, i)) {
				watchFiles[filepath.Join(root.Name(), filepath.FromSlash(f.path))] = true
			}
		}
	}

	for _, f := range files {
		if f.err != nil || !f.isDir() {
			continue
		}

		name := filepath.Join(root.Name(), filepath.FromSlash(f.path))
		for _, base := range cfg.bases() {
			rel, inside := within(f.path, base)
			_, ancestor := within(base, f.path)
			for _, pattern := range patterns {
				if ancestor || (inside && (rel == "." || pattern.mayContain(rel))) {
					watchDirs[name] = true
				}
			}
		}
	}

	return slices.Sorted(maps.Keys(watchFiles)), slices.Sorted(maps.Keys(watchDirs)), selectionErr
}
