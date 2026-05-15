package build

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// rollupTask concatenates scripts in declaration order. It never consumes a
// bundle's own output or that output's minified companion. Minification of
// bundles is a separate stage.
type rollupTask struct {
	addOnPaths
	entries []rollupEntry
}

// run writes each rollup before expanding the next so later rollups can consume
// earlier outputs.
func (t rollupTask) run(ctx context.Context, root *os.Root, files []file) error {
	files = slices.Clone(files)
	outputs := rollupOutputs(t.entries)

	for i, entry := range t.entries {
		var joined bytes.Buffer
		for _, pattern := range entry.inputs {
			matches, err := t.rollupFiles(ctx, files, pattern, outputs, i)
			if err != nil {
				return fmt.Errorf("expanding rollup input %q: %w", pattern.value, err)
			}

			if len(matches) == 0 {
				return fmt.Errorf("rollup input %q matched no files", pattern.value)
			}

			for _, f := range matches {
				code, err := readFile(ctx, root, f.path)
				if err != nil {
					return fmt.Errorf("reading rollup input %s: %w", f.path, err)
				}

				joined.Write(code)
				joined.WriteString("\n\n")
			}
		}

		if err := writeJavaScript(ctx, root, entry.output, bytes.TrimSpace(joined.Bytes())); err != nil {
			return fmt.Errorf("writing rollup %s: %w", entry.output, err)
		}

		info, err := root.Stat(filepath.FromSlash(entry.output))
		if err != nil {
			return fmt.Errorf("reading completed rollup %s: %w", entry.output, err)
		}

		index := slices.IndexFunc(files, func(f file) bool { return f.path == entry.output })
		output := file{path: entry.output, info: info}
		if index < 0 {
			files = append(files, output)
		} else {
			files[index] = output
		}
	}

	return nil
}
