package worktree

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// managementSource returns the source checkout for commands run in cwd. It is
// like sourceCheckout, but for an adopted checkout it returns the recorded
// source after confirming that the checkout still belongs to it.
func managementSource(ctx context.Context, cwd string) (string, error) {
	source, err := sourceCheckout(ctx, cwd)
	if err != nil {
		return "", err
	}

	if !pathWithin(source, cwd) {
		return source, nil
	}

	reg, err := newRegistry()
	if err != nil {
		return source, nil
	}

	found, ok, err := reg.forWorktree(source)
	if errors.Is(err, errRegistryAmbiguous) {
		return "", err
	}

	if err != nil || !ok {
		return source, nil
	}

	if err := validateAdoptedSource(ctx, source, found.SourcePath); err != nil {
		return "", fmt.Errorf("invalid recorded worktree source: %w", err)
	}

	return found.SourcePath, nil
}

// findWorktree returns the worktree of source for ref. A branch or workspace
// match in the worktrees directory comes first, then a registered worktree,
// then a directory name match.
func findWorktree(ctx context.Context, source, ref string) (match, error) {
	located, locateErr := locate(ctx, source, ref)
	if locateErr != nil && !errors.Is(locateErr, errWorktreeNotFound) {
		return match{}, locateErr
	}

	if locateErr == nil && !located.byName {
		return located, nil
	}

	recorded, err := recordedWorktree(source, ref)
	if err != nil {
		return match{}, err
	}

	if recorded != "" {
		return match{path: recorded}, nil
	}

	return located, locateErr
}

// recordedWorktree returns the path of the registered worktree of source that
// has branch checked out, or "" if there is none or the registry is
// unreadable. It returns an error if several worktrees match.
func recordedWorktree(source, branch string) (string, error) {
	reg, err := newRegistry()
	if err != nil {
		return "", nil
	}

	entries, err := reg.forSource(source)
	if err != nil {
		return "", nil
	}

	var matches []string

	seen := make(map[string]bool)

	for _, e := range entries {
		if e.Branch != branch {
			continue
		}

		id := resolveSymlinks(e.WorktreePath)
		if seen[id] {
			continue
		}

		seen[id] = true

		matches = append(matches, e.WorktreePath)
	}

	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0], nil
	}

	slices.Sort(matches)

	return "", fmt.Errorf(
		"several registered worktrees use the branch or workspace %q:\n  %s\nRun 'xf worktree prune' to drop stale entries, or correct the registry if an entry is wrong",
		branch, strings.Join(matches, "\n  "),
	)
}

// pathWithin reports whether target is base or inside it, after resolving
// symlinks.
func pathWithin(base, target string) bool {
	rel, err := filepath.Rel(resolveSymlinks(base), resolveSymlinks(target))
	if err != nil {
		return false
	}

	return filepath.IsLocal(rel)
}
