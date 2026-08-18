package worktree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/xf"
)

// instanceHashLength is the number of hex characters of the source-and-branch
// digest kept in a derived instance name. Twelve characters (48 bits) put an
// accidental collision between two checkouts or branches out of reach.
const instanceHashLength = 12

var (
	// ErrInvalidBranch indicates a branch name that cannot be used.
	ErrInvalidBranch = errors.New("invalid branch name")

	// ErrBranchExists indicates the branch is already present.
	ErrBranchExists = errors.New("branch already exists")

	// ErrWorktreeExists indicates the target directory is already in use.
	ErrWorktreeExists = errors.New("worktree already exists")

	// ErrNotXenForo indicates the source is not a XenForo installation.
	ErrNotXenForo = errors.New("not a XenForo directory")
)

// Options describes a worktree to create.
type Options struct {
	// SourcePath is the checkout to branch from.
	SourcePath string

	// Branch is the branch (Git) or workspace (Jujutsu) to create in the new
	// checkout. It also names the new directory.
	Branch string

	// Base is the revision to base the new checkout on, interpreted with the
	// selected backend's revision syntax. Defaults to the source's current HEAD.
	Base string

	// Instance overrides the derived Docker instance name.
	Instance string
}

// Result describes a created worktree.
type Result struct {
	// Path is the worktree location.
	Path string

	// Branch is the branch checked out in the worktree.
	Branch string

	// SourcePath is the checkout it was created from.
	SourcePath string

	// SourceBranch is the branch the source was on at creation time.
	SourceBranch string

	// Instance is the Docker instance name for the worktree.
	Instance string

	// CreatedAt is when the worktree was created.
	CreatedAt time.Time
}

// Preflight validates a request without changing anything.
//
// It runs before any mutation so that a rejected request leaves no partial
// state behind: no directory, no branch, no registry entry.
func Preflight(ctx context.Context, sourcePath, branch string) error {
	b, err := detectBackend(ctx, sourcePath)
	if err != nil {
		return err
	}

	return preflight(ctx, b, sourcePath, branch)
}

// preflight is Preflight with the backend already selected.
func preflight(ctx context.Context, b backend, sourcePath, branch string) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("%w: branch name is empty", ErrInvalidBranch)
	}

	dirName := BranchToDirName(branch)
	if dirName == "" {
		return fmt.Errorf("%w: %q does not yield a usable directory name", ErrInvalidBranch, branch)
	}

	xfPath := filepath.Join(sourcePath, "src", "XF.php")
	if _, err := os.Stat(xfPath); err != nil {
		return fmt.Errorf("%w: src/XF.php not found in %s", ErrNotXenForo, sourcePath)
	}

	exists, err := b.nameExists(ctx, sourcePath, branch)
	if err != nil {
		return err
	}

	if exists {
		return fmt.Errorf("%w: %s", ErrBranchExists, branch)
	}

	// Only the last segment of a branch names the directory, so different
	// branches can want the same one. Reject the collision rather than renaming
	// around it: an auto-generated suffix would make the path depend on what
	// existed at the time, and resolving a branch to its path would no longer
	// be possible without consulting stored state.
	target := filepath.Join(WorktreesDir(sourcePath), dirName)
	if _, err := os.Stat(target); err == nil {
		owner, ownerErr := b.owner(ctx, sourcePath, target)
		if ownerErr == nil && owner != "" && owner != branch {
			return fmt.Errorf(
				"%w: %q is already used by branch %q; choose a more specific final segment, such as %q",
				ErrWorktreeExists, dirName, owner, suggestAlternative(branch),
			)
		}

		return fmt.Errorf("%w: %s", ErrWorktreeExists, target)
	}

	return nil
}

// InstanceName derives the Docker Compose project name for a worktree.
//
// It combines a readable prefix taken from the branch's final segment with a
// short digest of the canonical source checkout and the full branch name. The
// branch-to-directory mapping is lossy - dev/24x/feature and dev/xfs/feature
// both reduce to "feature" - so without the digest those worktrees would also
// share a Compose project, and stopping or destroying one would affect the
// other. Mixing in the source path keeps checkouts that happen to use the same
// branch names apart as well.
//
// The digest is computed from the symlink-resolved source so that the name is
// stable whether the checkout is reached through a symlinked path or not.
func InstanceName(sourcePath, branch string) string {
	prefix := xf.GenerateInstanceName(BranchToDirName(branch))

	sum := sha256.Sum256([]byte(resolveSymlinks(sourcePath) + "\x00" + branch))
	suffix := hex.EncodeToString(sum[:])[:instanceHashLength]

	// Reserve room for the separator and the digest, shortening only the
	// readable prefix: truncating the digest would trade away the collision
	// resistance it exists to provide.
	maxPrefix := xf.MaxInstanceNameLength - len(suffix) - 1
	if len(prefix) > maxPrefix {
		prefix = strings.Trim(prefix[:maxPrefix], "-")
	}

	if prefix == "" {
		prefix = "xf"
	}

	return prefix + "-" + suffix
}

// Create makes a worktree on a new branch.
//
// It does not configure Docker or install anything; that is the caller's job.
// Pre-flight runs first, so a failure here leaves the source checkout untouched.
func Create(ctx context.Context, opts Options) (*Result, error) {
	b, source, err := resolveSource(ctx, opts.SourcePath)
	if err != nil {
		return nil, err
	}

	if err := preflight(ctx, b, source, opts.Branch); err != nil {
		return nil, err
	}

	sourceBranch, err := b.currentName(ctx, source)
	if err != nil {
		return nil, err
	}

	target := ResolvePath(source, opts.Branch)

	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create worktrees directory: %w", err)
	}

	if err := b.create(ctx, source, opts.Branch, opts.Base, target); err != nil {
		// Leave no empty container directory behind after a failure.
		_ = os.Remove(filepath.Dir(target))

		return nil, err
	}

	instance := opts.Instance
	if instance == "" {
		instance = InstanceName(source, opts.Branch)
	}

	return &Result{
		Path:         target,
		Branch:       opts.Branch,
		SourcePath:   source,
		SourceBranch: sourceBranch,
		Instance:     instance,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// suggestAlternative proposes a more specific name for a colliding branch, by
// including the segment before the last one.
func suggestAlternative(branch string) string {
	segments := strings.Split(strings.Trim(branch, "/"), "/")
	if len(segments) < 2 {
		return branch + "-2"
	}

	return segments[len(segments)-2] + "-" + segments[len(segments)-1]
}
