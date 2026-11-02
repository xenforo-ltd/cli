package worktree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// errNotARepository indicates a path is not inside a supported repository.
var errNotARepository = errors.New("not a git or Jujutsu repository")

// repositoryKind identifies the version control system a directory belongs to.
type repositoryKind int

const (
	// repositoryNone means no repository marker was found.
	repositoryNone repositoryKind = iota

	// repositoryGit means a .git marker was found.
	repositoryGit

	// repositoryJujutsu means a .jj marker was found.
	repositoryJujutsu
)

// repositoryMarker returns the version control system whose marker is closest
// to dir, walking from dir to its ancestors.
//
// Jujutsu is checked before Git at every level so a colocated repository -- one
// carrying both .jj and .git metadata -- is treated as Jujutsu. Detection is
// structural: the marker is only located, never interpreted, so a missing or
// misconfigured VCS tool cannot make a repository look like a different one.
// The marker is matched as a file or a directory because a linked Git worktree
// records .git as a file pointing at the shared repository.
func repositoryMarker(ctx context.Context, dir string) (repositoryKind, error) {
	markers := [...]struct {
		name string
		kind repositoryKind
	}{
		{".jj", repositoryJujutsu},
		{".git", repositoryGit},
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return repositoryNone, fmt.Errorf("failed to resolve repository path %s: %w", dir, err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return repositoryNone, err
		}

		for _, m := range markers {
			exists, err := markerExists(filepath.Join(abs, m.name))
			if err != nil {
				return repositoryNone, err
			}

			if exists {
				return m.kind, nil
			}
		}

		parent := filepath.Dir(abs)
		if parent == abs {
			return repositoryNone, nil
		}

		abs = parent
	}
}

// markerExists reports whether path exists, propagating inspection failures
// other than "not found" so a permission or I/O error is never mistaken for an
// absent repository.
func markerExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("failed to inspect %s: %w", path, err)
}

// backend is the version control system seam behind worktree orchestration.
//
// Every method is a single VCS operation that creating, inspecting or removing
// a checkout needs. Keeping the interface private lets the package present one
// stable, VCS-neutral API -- create, status, remove and the rest -- while Git
// and Jujutsu each implement the operations in their own terms: a Git branch
// and worktree, or a Jujutsu workspace.
// sourceCheckout resolves the original checkout, nameExists checks a branch or
// workspace name, currentName identifies the current one, and checkouts lists
// all registered checkouts. create performs only the VCS mutation, while status
// inspects potential data loss and remove deletes the checkout and its branch
// or workspace. Callers validate paths and perform removal safety checks.
type backend interface {
	sourceCheckout(ctx context.Context, dir string) (string, error)

	nameExists(ctx context.Context, repoDir, name string) (bool, error)

	currentName(ctx context.Context, checkoutPath string) (string, error)

	checkouts(ctx context.Context, repoDir string) ([]checkout, error)

	create(ctx context.Context, source, name, base, target string) error

	status(ctx context.Context, checkoutPath string) (worktreeStatus, error)

	remove(ctx context.Context, source, checkoutPath string, force bool) error
}

// checkout is a checkout registered with a repository: its root directory and
// the branch (Git) or workspace (Jujutsu) it has checked out. name is empty for
// a Git worktree on a detached HEAD.
type checkout struct {
	path string
	name string
}

// owner returns the branch or workspace checked out at checkoutPath, or an
// empty string when no checkout is registered there.
func owner(ctx context.Context, b backend, repoDir, checkoutPath string) (string, error) {
	want, err := filepath.Abs(checkoutPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s: %w", checkoutPath, err)
	}

	want = resolveSymlinks(want)

	checkouts, err := b.checkouts(ctx, repoDir)
	if err != nil {
		return "", err
	}

	for _, c := range checkouts {
		if resolveSymlinks(c.path) == want {
			return c.name, nil
		}
	}

	return "", nil
}

// detectBackend selects the backend for the repository containing dir.
//
// The repository's metadata decides the backend: .jj means Jujutsu, including
// in a repository colocated with Git, and .git means Git. Once a marker is
// found the matching backend is returned unconditionally, so a repository whose
// tooling is missing or broken surfaces that failure from the operation instead
// of quietly falling through to the other VCS. A directory with neither marker
// yields the repository error.
func detectBackend(ctx context.Context, dir string) (backend, error) {
	kind, err := repositoryMarker(ctx, dir)
	if err != nil {
		return nil, err
	}

	switch kind {
	case repositoryJujutsu:
		return jjBackend{}, nil
	case repositoryGit:
		return gitBackend{}, nil
	default:
		return nil, fmt.Errorf("%w: %s", errNotARepository, dir)
	}
}

// resolveSource detects the backend for dir and resolves the canonical source
// checkout, so a single operation probes the VCS once and reuses the backend.
func resolveSource(ctx context.Context, dir string) (backend, string, error) {
	b, err := detectBackend(ctx, dir)
	if err != nil {
		return nil, "", err
	}

	source, err := b.sourceCheckout(ctx, dir)
	if err != nil {
		return nil, "", err
	}

	return b, source, nil
}

// sourceCheckout returns the main checkout for the repository containing dir.
//
// When dir is inside a linked worktree or workspace this returns the *original*
// checkout, not the linked one. That keeps new checkouts siblings of the source
// rather than nesting them, so running the command from within one behaves the
// same as running it from the source.
func sourceCheckout(ctx context.Context, dir string) (string, error) {
	_, source, err := resolveSource(ctx, dir)

	return source, err
}

// resolveSymlinks resolves a path for comparison, falling back to the cleaned
// path when it cannot be resolved. Temporary directories on macOS are symlinked
// via /var, so comparing unresolved paths gives false mismatches.
func resolveSymlinks(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}

	return filepath.Clean(path)
}

// vcsOutput runs a version control tool in dir and returns its trimmed
// standard output. A cancelled context is reported in place of the exec error.
func vcsOutput(ctx context.Context, tool, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir

	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}

		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// vcsRun runs a version control tool in dir for its side effects. On failure
// the error names action and carries the tool's combined output; a cancelled
// context is reported in place of both.
func vcsRun(ctx context.Context, tool, dir, action string, args ...string) error {
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		return fmt.Errorf("failed to %s: %w: %s", action, err, strings.TrimSpace(string(out)))
	}

	return nil
}

// nonEmptyLines returns the trimmed, non-blank lines of s.
func nonEmptyLines(s string) []string {
	var lines []string

	for line := range strings.SplitSeq(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	return lines
}
