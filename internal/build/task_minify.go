package build

import (
	"context"
	"fmt"
	"os"
)

// minifyTask minifies JavaScript files matching the configured patterns. It runs
// after rollups, so matched rollup outputs are minified alongside sources.
// outputs lists declared rollup outputs, which take precedence over _files.
type minifyTask struct {
	addOnPaths
	patterns []filePattern
	outputs  []string
}

// run expands patterns and minifies each installation destination at most once.
func (t minifyTask) run(ctx context.Context, root *os.Root, files []file) error {
	seen := make(map[string]bool)
	for _, pattern := range t.patterns {
		matches, err := t.minifyFiles(ctx, files, pattern, t.outputs)
		if err != nil {
			return fmt.Errorf("expanding minify pattern %q: %w", pattern.value, err)
		}

		for _, f := range matches {
			key := t.installationPath(f.path)
			if seen[key] {
				continue
			}

			seen[key] = true
			if err := minifyFile(ctx, root, f.path); err != nil {
				return err
			}
		}
	}

	return nil
}
