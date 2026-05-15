package build

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
)

// uploadSelection selects sources and maps them to installation-relative destinations.
type uploadSelection struct {
	addOnPaths
	whitelist        []filePattern
	blacklist        []filePattern
	additional       []string
	composerAutoload string
}

// selectFiles returns installation-relative destinations mapped to
// build-root-relative source paths.
func (s uploadSelection) selectFiles(ctx context.Context, files []file) (map[string]string, error) {
	selected, err := s.selectWhitelist(ctx, files)
	if err != nil {
		return nil, err
	}

	if err := s.addAdditionalFiles(ctx, files, selected); err != nil {
		return nil, err
	}
	if s.composerAutoload != "" {
		installed := path.Join(s.prefix, s.addOnDir, s.composerAutoload, "installed.json")
		if _, ok := selected[installed]; !ok {
			return nil, fmt.Errorf("addon.json composer_autoload requires %s in the release; check installed dependencies and file selection", installed)
		}
	}

	return selected, nil
}

// selectWhitelist selects files allowed by the whitelist, blacklist, and built-in
// exclusions. The add-on metadata must be selected.
func (s uploadSelection) selectWhitelist(ctx context.Context, files []file) (map[string]string, error) {
	selected := make(map[string]string)
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if isIgnoredPath(f.path) || excluded(s.blacklist, f.path, f.isDir()) ||
			(isHiddenPath(f.path) && !s.matchesWhitelist(f.path, f.isDir() || f.err != nil)) {
			continue
		}

		if f.err != nil {
			for _, pattern := range s.whitelist {
				if pattern.allowsHidden(f.path) && (f.path == "." || pattern.matches(f.path, false) || pattern.mayContain(f.path)) {
					return nil, fmt.Errorf("reading %s: %w", f.path, f.err)
				}
			}
		}

		if f.isDir() || !s.matchesWhitelist(f.path, false) {
			continue
		}

		if err := f.regular(); err != nil {
			return nil, err
		}

		selected[path.Join(s.prefix, f.path)] = f.path
	}

	metadata := path.Join(s.prefix, s.addOnDir, "addon.json")
	if _, ok := selected[metadata]; !ok {
		return nil, fmt.Errorf("%s must be included by the whitelist and not excluded by the blacklist", metadata)
	}

	return selected, nil
}

// addAdditionalFiles adds requested files and directory contents to selected,
// searching _files before the build root. These entries are not subject to the
// blacklist and may replace whitelist selections.
func (s uploadSelection) addAdditionalFiles(ctx context.Context, files []file, selected map[string]string) error {
	for _, entry := range s.additional {
		if err := ctx.Err(); err != nil {
			return err
		}

		if isIgnoredPath(entry) || (isHiddenPath(entry) && !s.matchesWhitelist(entry, true)) {
			continue
		}

		resolved := false
		for _, base := range s.bases() {
			source := path.Join(base, entry)
			for _, f := range files {
				if f.path == source {
					if f.err != nil {
						return fmt.Errorf("reading additional file %s: %w", entry, f.err)
					}

					resolved = true
					break
				}
			}

			if !resolved {
				continue
			}

			for _, f := range files {
				rel, ok := within(f.path, source)
				name := path.Join(entry, rel)
				if !ok || isIgnoredPath(rel) || (isHiddenPath(name) && !s.matchesWhitelist(name, f.isDir() || f.err != nil)) {
					continue
				}

				if f.err != nil {
					return fmt.Errorf("reading additional file %s: %w", f.path, f.err)
				}

				if f.isDir() {
					continue
				}

				if err := f.regular(); err != nil {
					return err
				}

				key := name
				if base == "." {
					key = path.Join(s.prefix, key)
				}

				selected[key] = f.path
			}

			break
		}

		if !resolved {
			return fmt.Errorf("reading additional file %s: %w", entry, os.ErrNotExist)
		}
	}

	return nil
}

// matchesWhitelist reports whether a whitelist pattern selects name, explicitly
// naming its hidden components. Directories may be ancestors of selected files.
func (s uploadSelection) matchesWhitelist(name string, isDir bool) bool {
	hidden := isHiddenPath(name)
	for _, pattern := range s.whitelist {
		if (pattern.matches(name, isDir) || (isDir && pattern.mayContain(name))) &&
			(!hidden || pattern.allowsHidden(name)) {
			return true
		}
	}

	return false
}

// isIgnoredPath reports whether rel contains reserved build files or directories.
func isIgnoredPath(rel string) bool {
	for part := range strings.SplitSeq(rel, "/") {
		switch part {
		case "_build", "_releases", "_files", "_no_upload", "_output", "_stubs":
			return true
		}
	}

	name := path.Base(rel)
	return name == "build.json" || name == "hashes.json"
}

// isHiddenPath reports whether name contains a dot-prefixed component other than ".".
func isHiddenPath(name string) bool {
	for part := range strings.SplitSeq(name, "/") {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}

	return false
}

// excluded reports whether name or a parent directory matches patterns.
func excluded(patterns []filePattern, name string, isDir bool) bool {
	if matchesPatterns(patterns, name, isDir) {
		return true
	}

	for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
		if matchesPatterns(patterns, parent, true) {
			return true
		}
	}

	return false
}
