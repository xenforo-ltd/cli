package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// BuildJavaScript runs the add-on's configured minify and rollup tasks.
//
// rootDir and addOnID have the same meaning as in [Build].
// It does not install dependencies, run configured commands, or create a release.
func BuildJavaScript(ctx context.Context, rootDir, addOnID string) error {
	installation, err := openInstallation(ctx, rootDir)
	if err != nil {
		return fmt.Errorf("opening XenForo root: %w", err)
	}

	defer installation.Close()
	cfg, root, err := loadBuildRoot(ctx, installation, addOnID)
	if err != nil {
		return err
	}

	defer root.Close()
	return runTasks(ctx, root, javascriptTasks(cfg), nil)
}

// javascriptTasks returns JavaScript build stages in execution order: rollups
// first, then minification. Minification always runs after rollups, so a bundle
// is built from full sources and its minified companion maps back to the
// readable bundle. Wildcard rollup inputs skip ".min.js" companions; minified
// files can still be listed explicitly.
func javascriptTasks(cfg *config) []task {
	var tasks []task
	if len(cfg.rollups) > 0 {
		tasks = append(tasks, rollupTask{addOnPaths: cfg.addOnPaths, entries: cfg.rollups})
	}

	if len(cfg.minify) > 0 {
		tasks = append(tasks, minifyTask{addOnPaths: cfg.addOnPaths, patterns: cfg.minify, outputs: rollupOutputs(cfg.rollups)})
	}

	return tasks
}

// collectJavaScriptFiles discovers only matching inputs and their ancestors in
// the build root and _files. Selection and precedence remain in scriptFiles.
func (p addOnPaths) collectJavaScriptFiles(ctx context.Context, root *os.Root, patterns []filePattern) ([]file, error) {
	return collectFilesWhere(ctx, root, ".", func(name string, _ bool) bool {
		for _, base := range p.bases() {
			rel, inside := within(name, base)
			if !inside {
				continue
			}
			if rel == "." {
				return true
			}
			for _, pattern := range patterns {
				if pattern.matches(rel, false) || pattern.mayContain(rel) {
					return true
				}
			}
		}
		return false
	})
}

// rollupOutputs returns output paths in declaration order.
func rollupOutputs(entries []rollupEntry) []string {
	outputs := make([]string, len(entries))
	for i, entry := range entries {
		outputs[i] = entry.output
	}

	return outputs
}

// minifiedPath returns the companion ".min.js" path for a ".js" path.
func minifiedPath(name string) string { return strings.TrimSuffix(name, ".js") + ".min.js" }

// declaredOutput reports whether name maps to a declared rollup destination.
func (p addOnPaths) declaredOutput(name string, outputs []string) bool {
	key := p.installationPath(name)
	for _, output := range outputs {
		if key == p.installationPath(output) {
			return true
		}
	}

	return false
}

// minifyInput reports whether minification should write a companion for name.
// Existing ".min.js" companions are skipped.
func minifyInput(name string) bool {
	return strings.HasSuffix(name, ".js") && !strings.HasSuffix(name, ".min.js")
}

// minifyFiles selects unminified JavaScript sources and matched rollup outputs.
func (p addOnPaths) minifyFiles(ctx context.Context, files []file, pattern filePattern, outputs []string) ([]file, error) {
	return p.scriptFiles(ctx, files, pattern, outputs, minifyInput)
}

// rollupFiles selects inputs for outputs[index]. The bundle's own output and
// minified companion are always excluded. Wildcards also skip minified files and
// later rollup outputs, which may be left over from a previous build.
func (p addOnPaths) rollupFiles(ctx context.Context, files []file, pattern filePattern, outputs []string, index int) ([]file, error) {
	output := p.installationPath(outputs[index])
	return p.scriptFiles(ctx, files, pattern, outputs[:index], func(name string) bool {
		key := p.installationPath(name)
		if key == output || key == minifiedPath(output) {
			return false
		}
		if isLiteral(pattern.value) {
			return true
		}

		return !strings.HasSuffix(name, ".min.js") && !p.declaredOutput(name, outputs[index+1:])
	})
}

// scriptFiles expands pattern against the lookup bases, filters inputs before
// validating them, and returns matches in installation-path order. _files
// sources take precedence, except that paths in outputs replace matching _files
// sources. On validation errors, all candidates are retained so watch mode can
// detect repairs to shadowed inputs as well.
func (p addOnPaths) scriptFiles(ctx context.Context, files []file, pattern filePattern, outputs []string, include func(string) bool) ([]file, error) {
	found := make(map[string]file)
	var candidates []file
	var selectionErr error
	for _, base := range p.bases() {
		for _, f := range files {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			rel, ok := within(f.path, base)
			if !ok {
				continue
			}

			matched := pattern.matches(rel, false)
			// Keep errors for ancestors of matching inputs, but ignore files
			// excluded by the stage's input rules.
			if !f.isDir() && !include(f.path) && (f.err == nil || matched) {
				continue
			}

			if f.err != nil && (matched || pattern.mayContain(rel) || rel == ".") && selectionErr == nil {
				selectionErr = fmt.Errorf("reading %s: %w", f.path, f.err)
			}

			if f.isDir() || !matched {
				continue
			}

			candidates = append(candidates, f)
			if err := f.regular(); err != nil && selectionErr == nil {
				selectionErr = err
			}

			key := p.installationPath(f.path)
			if _, exists := found[key]; !exists || slices.Contains(outputs, f.path) {
				found[key] = f
			}
		}
	}

	if selectionErr != nil {
		return candidates, selectionErr
	}

	keys := slices.Sorted(maps.Keys(found))
	result := make([]file, 0, len(keys))
	for _, key := range keys {
		result = append(result, found[key])
	}

	return result, nil
}

// minifyFile minifies source and writes its companion ".min.js" file and
// external ".min.js.map" source map.
func minifyFile(ctx context.Context, root *os.Root, source string) error {
	code, err := readFile(ctx, root, source)
	if err != nil {
		return fmt.Errorf("reading %s: %w", source, err)
	}

	minified, err := minifyJavaScript(ctx, source, string(code))
	if err != nil {
		return fmt.Errorf("minifying %s: %w", source, err)
	}

	name := minifiedPath(source)
	code = minified.code
	if len(minified.sourceMap) > 0 {
		code = linkSourceMap(name, code)
	}

	if err := writeJavaScript(ctx, root, name, code); err != nil {
		return fmt.Errorf("writing minified %s: %w", source, err)
	}

	if len(minified.sourceMap) > 0 {
		if err := writeJavaScript(ctx, root, name+".map", minified.sourceMap); err != nil {
			return fmt.Errorf("writing source map for %s: %w", source, err)
		}
	}

	return nil
}

// javascriptOutput holds minified code and its external source map.
type javascriptOutput struct {
	code      []byte
	sourceMap []byte
}

// linkSourceMap appends a sourceMappingURL comment referencing an external map
// beside name.
func linkSourceMap(name string, code []byte) []byte {
	linked := make([]byte, 0, len(code)+len(name)+32)
	linked = append(linked, code...)
	if len(code) > 0 && code[len(code)-1] != '\n' {
		linked = append(linked, '\n')
	}

	return append(linked, "//# sourceMappingURL="+path.Base(name)+".map\n"...)
}

// minifyJavaScript transforms and minifies code for ES2018, matching the
// Closure Compiler language output the existing build uses, and produces an
// external source map without embedded source contents. Loader, platform,
// legal comments, and tree shaking keep esbuild's defaults; the transform API
// never bundles, so tree shaking does not apply. An in-flight transform may
// continue after ctx is canceled.
func minifyJavaScript(ctx context.Context, sourcefile, code string) (javascriptOutput, error) {
	if err := ctx.Err(); err != nil {
		return javascriptOutput{}, err
	}

	completed := make(chan api.TransformResult, 1)
	go func() {
		completed <- api.Transform(code, api.TransformOptions{
			// The source and map are siblings, so the map needs only the basename.
			Sourcefile:        path.Base(sourcefile),
			Target:            api.ES2018,
			Sourcemap:         api.SourceMapExternal,
			SourcesContent:    api.SourcesContentExclude,
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			MinifySyntax:      true,
		})
	}()

	var result api.TransformResult
	select {
	case result = <-completed:
	case <-ctx.Done():
		return javascriptOutput{}, ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		return javascriptOutput{}, err
	}

	if len(result.Errors) > 0 {
		return javascriptOutput{}, errors.New(result.Errors[0].Text)
	}

	return javascriptOutput{code: result.Code, sourceMap: result.Map}, nil
}

// writeJavaScript replaces name only if its contents differ from data.
func writeJavaScript(ctx context.Context, root *os.Root, name string, data []byte) error {
	previous, err := readFile(ctx, root, name)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err == nil && bytes.Equal(previous, data) {
		return nil
	}

	return writeFile(ctx, root, name, data)
}
