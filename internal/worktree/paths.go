// Package worktree manages git worktrees for XenForo development environments.
package worktree

import (
	"fmt"
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
// Only the last segment is used, so dev/xfs/slack-unfurl becomes slack-unfurl.
// The prefix in a conventional branch name describes where the work belongs
// rather than what it is, and repeating it in directory names, URLs and Docker
// instance names makes all three harder to read.
//
// The result is always a single segment: it can never contain a separator or
// resolve to a parent directory, whatever the branch name contains.
//
// This is deliberately lossy. dev/xfs/slack-unfurl and dev/xf/slack-unfurl both
// yield slack-unfurl, so callers must check whether the directory is already
// taken. Preflight rejects a collision rather than renaming around it, which
// keeps the branch-to-path mapping computable without consulting any state.
func BranchToDirName(branch string) string {
	// Take the last non-empty segment, so a trailing slash does not produce an
	// empty name.
	segments := strings.Split(branch, "/")

	last := ""

	for i := len(segments) - 1; i >= 0; i-- {
		if strings.TrimSpace(segments[i]) != "" {
			last = segments[i]

			break
		}
	}

	if last == "" {
		last = branch
	}

	name := unsafeChars.ReplaceAllString(last, "-")

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

// ResolveExistingPath is ResolvePath for branch names that came from the user.
//
// BranchToDirName yields an empty name for inputs such as "." or "..", which
// would resolve to the directory holding every worktree for the checkout. A
// command acting on that path would operate on all of them at once, so those
// inputs are rejected rather than resolved.
func ResolveExistingPath(sourcePath, branch string) (string, error) {
	if BranchToDirName(branch) == "" {
		return "", fmt.Errorf("%w: %q does not name a worktree", ErrInvalidBranch, branch)
	}

	return ResolvePath(sourcePath, branch), nil
}
