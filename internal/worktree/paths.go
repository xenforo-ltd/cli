// Package worktree manages Git worktrees and Jujutsu workspaces for XenForo
// development environments.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/xenforo-ltd/cli/internal/xf"
)

// worktreesSuffix is appended to a checkout's directory name to form the
// directory that holds its worktrees.
const worktreesSuffix = ".worktrees"

// unsafeChars matches anything not allowed in a worktree name. Names are used
// as directory names, Docker Compose project names and hostnames, so only the
// characters valid in all three survive; everything else becomes a separator.
var unsafeChars = regexp.MustCompile(`[^a-z0-9]+`)

// maxNameAttempts bounds the search for a free numbered name. Reaching it means
// something is wrong with the availability check rather than that a user has
// a thousand worktrees for one branch name.
const maxNameAttempts = 1000

// BaseName converts a branch name into the preferred name for its worktree.
//
// Only the last segment is used, so dev/xfs/slack-unfurl becomes slack-unfurl.
// The prefix in a conventional branch name describes where the work belongs
// rather than what it is, and repeating it in directory names, URLs and Docker
// instance names makes all three harder to read.
//
// The result is lowercase letters, digits and single dashes, so it is safe as a
// path segment, a Compose project name and a hostname label at once. It can
// never contain a separator or resolve to a parent directory, whatever the
// branch name contains. An empty result means the branch yields no usable name.
//
// This is deliberately lossy: dev/xfs/slack-unfurl and dev/xf/slack-unfurl
// both yield slack-unfurl. ChooseName resolves that by numbering.
func BaseName(branch string) string {
	// Take the last segment that yields a name, so a trailing slash or a
	// segment of only punctuation does not produce an empty one.
	for _, segment := range slices.Backward(strings.Split(branch, "/")) {
		name := strings.Trim(unsafeChars.ReplaceAllString(strings.ToLower(segment), "-"), "-")
		if name != "" {
			return truncateName(name, xf.MaxInstanceNameLength)
		}
	}

	return ""
}

// numberedName returns the nth candidate for base: base itself first, then
// base-2, base-3 and so on. The base is shortened when needed so the suffix
// always fits within the length limit.
func numberedName(base string, n int) string {
	if n <= 1 {
		return base
	}

	suffix := "-" + strconv.Itoa(n)

	return truncateName(base, xf.MaxInstanceNameLength-len(suffix)) + suffix
}

// truncateName shortens name to at most limit characters, cutting at a word
// boundary where there is one so that a name stays readable: a long branch
// becomes "user-state-ladder" rather than "user-state-ladder-r".
func truncateName(name string, limit int) string {
	if len(name) <= limit {
		return name
	}

	cut := name[:limit]

	// A cut that lands exactly before a dash already ends on a whole word.
	if name[limit] != '-' {
		if i := strings.LastIndex(cut, "-"); i > 0 {
			cut = cut[:i]
		}
	}

	return strings.Trim(cut, "-")
}

// ChooseName picks the name for a new worktree of branch.
//
// The name is BaseName(branch), numbered -2, -3 and so on when that is taken.
// A name is taken when the checkout's worktrees directory already holds it or
// when taken reports it in use, which lets the caller check state this package
// does not own, such as Docker Compose projects. taken may be nil.
//
// The result depends on what exists at the time, so it cannot be recomputed
// from the branch later. It does not need to be: the name is recorded in the
// worktree's environment and the registry, and Locate finds a worktree from
// its branch through the VCS.
func ChooseName(sourcePath, branch string, taken func(name string) (bool, error)) (string, error) {
	base := BaseName(branch)
	if base == "" {
		return "", fmt.Errorf("%w: %q does not yield a usable worktree name", ErrInvalidBranch, branch)
	}

	dir := WorktreesDir(sourcePath)

	for n := 1; n <= maxNameAttempts; n++ {
		name := numberedName(base, n)

		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("failed to inspect %s: %w", filepath.Join(dir, name), err)
		}

		if taken != nil {
			inUse, err := taken(name)
			if err != nil {
				return "", err
			}

			if inUse {
				continue
			}
		}

		return name, nil
	}

	return "", fmt.Errorf("%w: no free name for %q after %d attempts", ErrWorktreeExists, base, maxNameAttempts)
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

// Locate finds an existing worktree of the checkout at sourcePath.
//
// ref may be the worktree's branch (or Jujutsu workspace) or its name, the
// directory it lives in. A branch match wins, so a branch that happens to be
// named like another worktree's directory still resolves to its own worktree.
// Only checkouts inside the worktrees directory are considered: the source
// checkout itself is never a match. Neither is a checkout whose directory was
// deleted without telling the VCS, which Git still lists as prunable.
//
// The path is returned under WorktreesDir(sourcePath), the form Create reports
// and the registry records, rather than as the VCS spells it, which may have
// symlinks resolved.
func Locate(ctx context.Context, sourcePath, ref string) (string, error) {
	b, err := detectBackend(ctx, sourcePath)
	if err != nil {
		return "", err
	}

	checkouts, err := b.checkouts(ctx, sourcePath)
	if err != nil {
		return "", err
	}

	dir := resolveSymlinks(WorktreesDir(sourcePath))

	var byName string

	for _, c := range checkouts {
		path := resolveSymlinks(c.path)
		if filepath.Dir(path) != dir {
			continue
		}

		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}

			return "", fmt.Errorf("failed to inspect worktree %s: %w", path, err)
		}

		name := filepath.Base(path)

		if c.name == ref {
			return filepath.Join(WorktreesDir(sourcePath), name), nil
		}

		if name == ref && byName == "" {
			byName = name
		}
	}

	if byName != "" {
		return filepath.Join(WorktreesDir(sourcePath), byName), nil
	}

	return "", fmt.Errorf("%w: no worktree for %q in %s", ErrWorktreeNotFound, ref, WorktreesDir(sourcePath))
}
