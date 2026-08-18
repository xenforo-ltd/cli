// Package worktree manages git worktrees for XenForo development environments.
package worktree

import (
	"path/filepath"
	"regexp"
	"strings"
)

// worktreesSuffix is appended to a checkout's directory name to form the
// directory that holds its worktrees.
const worktreesSuffix = ".worktrees"

// unsafeChars matches anything not allowed in a worktree directory name.
// Letters, digits, dots, underscores and dashes are kept; everything else
// becomes a separator.
var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// BranchToDirName converts a branch name into a single, safe path segment.
//
// Slashes become dashes, so dev/24x/feature becomes dev-24x-feature. The result
// is always a single segment: it can never contain a separator or resolve to a
// parent directory, whatever the branch name contains.
//
// Note this is lossy. dev/24x/feature and dev-24x-feature both map to
// dev-24x-feature, so callers must check for an existing directory rather than
// assume the name is unique. See CheckCollision.
func BranchToDirName(branch string) string {
	name := unsafeChars.ReplaceAllString(branch, "-")

	// Leading dots would create a hidden directory, and a name of only dots
	// would resolve to "." or "..".
	name = strings.Trim(name, "-")
	name = strings.TrimLeft(name, ".")
	name = strings.Trim(name, "-")

	// Collapse runs introduced by the substitutions above.
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	if name == "." || name == ".." {
		return ""
	}

	return name
}

// WorktreesDir returns the directory holding the worktrees for a checkout.
//
// Worktrees are siblings of the source checkout, so ~/Sites/main yields
// ~/Sites/main.worktrees. This keeps them on the same filesystem as the source,
// which matters for Docker bind mounts, and makes them discoverable without
// knowing an xf-specific convention.
func WorktreesDir(sourcePath string) string {
	cleaned := filepath.Clean(sourcePath)

	return cleaned + worktreesSuffix
}

// ResolvePath returns the worktree path for a branch of the given checkout.
//
// The result depends only on its arguments, so any tool can predict it without
// consulting the registry or git.
func ResolvePath(sourcePath, branch string) string {
	return filepath.Join(WorktreesDir(sourcePath), BranchToDirName(branch))
}
