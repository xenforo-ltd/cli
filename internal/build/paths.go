package build

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// pathKind selects validation rules for a configured path.
type pathKind uint8

const (
	// literalPath identifies a literal file or directory path beneath the build root.
	literalPath pathKind = iota
	// globPath identifies a relative glob, allowing backslashes for escaping.
	globPath
	// rootPath identifies a build root path, allowing parent directory references.
	rootPath
)

// relativePath validates value for kind and normalizes its spelling.
// Globs retain their internal path syntax; other paths are cleaned.
func relativePath(value string, kind pathKind) (string, error) {
	glob, parents := kind == globPath, kind == rootPath
	if value == "" || path.IsAbs(value) || strings.ContainsAny(value, ":\x00") || (!glob && strings.Contains(value, `\`)) {
		return "", fmt.Errorf("path %q must be relative and use forward slashes", value)
	}

	if !parents {
		for part := range strings.SplitSeq(value, "/") {
			if part == ".." {
				return "", fmt.Errorf("path %q must not contain parent directory references", value)
			}
		}
	}

	if glob {
		for strings.HasPrefix(value, "./") {
			value = strings.TrimPrefix(value, "./")
		}

		return value, nil
	}

	value = path.Clean(value)
	if kind == literalPath {
		if _, err := filepath.Localize(value); err != nil {
			return "", fmt.Errorf("invalid path %q: %w", value, err)
		}
	}

	return value, nil
}

// filePattern is a validated glob and its literal directory prefix.
type filePattern struct {
	value  string
	prefix string
}

// parsePatterns validates relative globs and removes leading "./" prefixes.
func parsePatterns(values []string) ([]filePattern, error) {
	patterns := make([]filePattern, 0, len(values))
	for _, value := range values {
		value, err := relativePath(value, globPath)
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(value, "!") {
			return nil, fmt.Errorf("pattern %q uses unsupported negation; use the blacklist to exclude files", value)
		}

		if !doublestar.ValidatePattern(value) {
			return nil, fmt.Errorf("pattern %q: %w", value, doublestar.ErrBadPattern)
		}

		prefix, _ := doublestar.SplitPattern(value)
		patterns = append(patterns, filePattern{value: value, prefix: prefix})
	}

	return patterns, nil
}

// matchesPatterns reports whether any pattern matches rel.
func matchesPatterns(patterns []filePattern, rel string, isDir bool) bool {
	for _, pattern := range patterns {
		if pattern.matches(rel, isDir) {
			return true
		}
	}

	return false
}

// matches reports whether p matches rel, including a trailing slash for directories.
func (p filePattern) matches(rel string, isDir bool) bool {
	if doublestar.MatchUnvalidated(p.value, rel) {
		return true
	}

	return isDir && doublestar.MatchUnvalidated(p.value, rel+"/")
}

// mayContain reports whether dir could contain a path matched by p.
func (p filePattern) mayContain(dir string) bool {
	// Escaped glob prefixes cannot be compared directly with disk paths.
	if strings.Contains(p.value, `\`) {
		return true
	}

	if p.prefix != "." && dir != p.prefix && !strings.HasPrefix(p.prefix, dir+"/") && !strings.HasPrefix(dir, p.prefix+"/") {
		return false
	}

	if strings.Contains(p.value, "**") || strings.ContainsAny(p.value, "{}") {
		return true
	}

	return strings.Count(dir, "/") < strings.Count(p.value, "/")
}

// allowsHidden reports whether p explicitly matches every hidden component in
// name with a dot-prefixed pattern component. Broad wildcards do not opt in.
func (p filePattern) allowsHidden(name string) bool {
	parts := strings.Split(name, "/")
	pattern := strings.Split(p.value, "/")
	for i, part := range parts {
		if !isHiddenPath(part) {
			continue
		}

		allowed := false
		for j, component := range pattern {
			if isHiddenPath(strings.TrimPrefix(component, `\`)) &&
				doublestar.MatchUnvalidated(strings.Join(pattern[:j+1], "/"), strings.Join(parts[:i+1], "/")) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	return true
}

// isLiteral reports whether value contains no glob metacharacters.
func isLiteral(value string) bool { return !strings.ContainsAny(value, "*?[{") }

// addOnPaths maps paths between the build root, its _files directory,
// and the installation. addOnDir is relative to the build root; prefix is
// relative to the installation.
type addOnPaths struct {
	addOnDir string
	prefix   string
}

// installationPath returns the installation-relative destination for name.
// Files beneath the build root's _files directory lose that directory prefix.
func (p addOnPaths) installationPath(name string) string {
	if rel, ok := within(name, "_files"); ok {
		return rel
	}

	return path.Join(p.prefix, name)
}

// bases returns lookup directories relative to the build root, in priority order.
func (p addOnPaths) bases() []string {
	return []string{"_files", "."}
}

// within returns name relative to base and reports whether name is within base.
// It expects cleaned, slash-separated relative paths.
func within(name, base string) (string, bool) {
	if base == "." {
		return name, true
	}

	if name == base {
		return ".", true
	}

	return strings.CutPrefix(name, base+"/")
}
