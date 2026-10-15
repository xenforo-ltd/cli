package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
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

// collectFiles discovers release files, pruning excluded directories, and maps
// installation-relative destinations to build-root-relative sources.
func (s uploadSelection) collectFiles(ctx context.Context, root *os.Root) (map[string]string, error) {
	files, err := collectFilesWhere(ctx, root, ".", func(name string, isDir bool) bool {
		return !isIgnoredPath(name) && !matchesPatterns(s.blacklist, name, isDir) && s.matchesWhitelist(name, isDir)
	})
	if err != nil {
		return nil, err
	}

	selected := make(map[string]string)
	for _, f := range files {
		if f.err != nil {
			return nil, fmt.Errorf("reading %s: %w", f.path, f.err)
		}
		if f.isDir() {
			continue
		}

		if err := f.regular(); err != nil {
			return nil, err
		}

		selected[path.Join(s.prefix, f.path)] = f.path
	}

	if err := s.addAdditionalFiles(ctx, root, selected); err != nil {
		return nil, err
	}

	// Required metadata bypasses release filters. Explicit additional-file
	// overrides still take precedence over the build-root copy.
	required := []string{path.Join(s.addOnDir, "addon.json")}
	if s.composerAutoload != "" {
		required = append(required, path.Join(s.addOnDir, s.composerAutoload, "installed.json"))
	}
	for _, name := range required {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := path.Join(s.prefix, name)
		if _, ok := selected[key]; ok {
			continue
		}
		info, err := root.Stat(filepath.FromSlash(name))
		if err := (file{path: name, info: info, err: err}).regular(); err != nil {
			return nil, fmt.Errorf("including required release file: %w", err)
		}
		selected[key] = name
	}

	return selected, nil
}

// addAdditionalFiles adds requested files and directory contents to selected,
// searching _files before the build root. These entries are not subject to the
// blacklist and may replace whitelist selections.
func (s uploadSelection) addAdditionalFiles(ctx context.Context, root *os.Root, selected map[string]string) error {
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
			if _, err := root.Lstat(filepath.FromSlash(source)); errors.Is(err, os.ErrNotExist) {
				continue
			} else if err != nil {
				return fmt.Errorf("reading additional file %s: %w", entry, err)
			}

			resolved = true
			files, err := collectFilesWhere(ctx, root, source, func(name string, isDir bool) bool {
				rel, _ := within(name, source)
				destination := path.Join(entry, rel)
				return !isIgnoredPath(rel) && (!isHiddenPath(destination) || s.matchesWhitelist(destination, isDir))
			})
			if err != nil {
				return err
			}
			for _, f := range files {
				rel, _ := within(f.path, source)
				name := path.Join(entry, rel)
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
