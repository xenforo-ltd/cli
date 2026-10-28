package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	// errInvalidPath indicates the supplied path does not name an adoptable
	// directory.
	errInvalidPath = errors.New("invalid worktree path")

	// errNotAWorktree indicates the path is not a registered linked checkout.
	errNotAWorktree = errors.New("not a registered worktree or workspace")

	// errNoBranch indicates a Git checkout has no branch to address it by.
	errNoBranch = errors.New("checkout has no branch")
)

// AdoptOptions describes an existing checkout to register.
type AdoptOptions struct {
	// Path is an absolute or relative path to a checkout's root directory or a
	// directory inside it. A relative path is resolved against the caller's
	// current directory.
	Path string

	// Instance overrides the derived Docker instance name. It is registry
	// metadata only; adoption never writes Docker configuration.
	Instance string
}

// Adopt registers an existing Git worktree or Jujutsu workspace that xf did
// not create.
//
// It is metadata-only: it discovers the checkout's root, source and name, and
// returns them as a Result. It does not touch checkout files, version-control
// metadata, working-copy state, branches, bookmarks or workspaces, and it does
// not configure Docker or run any environment setup. Writing the registry
// record is the caller's job.
//
// The path may point at the checkout root or a directory inside it. The
// checkout must belong to a supported repository, must be a XenForo checkout
// with a regular src/XF.php, and must not be the source or main checkout.
// A detached Git checkout is refused; a Jujutsu workspace named HEAD is not
// detached and remains valid.
func Adopt(ctx context.Context, opts AdoptOptions) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// A blank path is a mistake, but an otherwise meaningful path is never
	// trimmed: leading or trailing spaces can be part of a directory name.
	if strings.TrimSpace(opts.Path) == "" {
		return nil, fmt.Errorf("%w: path is empty", errInvalidPath)
	}

	abs, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", opts.Path, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s does not exist", errInvalidPath, opts.Path)
		}

		return nil, fmt.Errorf("failed to inspect %s: %w", opts.Path, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", errInvalidPath, opts.Path)
	}

	// Symlinks are resolved before detection so an alias outside the checkout
	// still finds the repository markers in its ancestors.
	canonicalDir, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", opts.Path, err)
	}

	b, err := detectBackend(ctx, canonicalDir)
	if err != nil {
		return nil, err
	}

	discovered, err := b.linkedCheckout(ctx, canonicalDir)
	if err != nil {
		return nil, err
	}

	root := resolveSymlinks(discovered.root)
	source := resolveSymlinks(discovered.source)

	// The source or main checkout already manages itself. Adopting it would
	// register the original checkout as one of its own linked checkouts.
	if root == source {
		return nil, fmt.Errorf(
			"%w: %s is the source checkout; adopt a linked worktree or workspace instead",
			errInvalidPath, root,
		)
	}

	if err := requireXenForoCheckout(root); err != nil {
		return nil, err
	}

	if strings.TrimSpace(discovered.name) == "" {
		return nil, fmt.Errorf("%w: could not determine the branch or workspace name at %s", errNotAWorktree, root)
	}

	sourceBranch, err := b.currentName(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("failed to determine the source branch or workspace: %w", err)
	}

	if strings.TrimSpace(sourceBranch) == "" {
		return nil, fmt.Errorf(
			"%w: could not determine the branch or workspace at the source checkout %s",
			errNotAWorktree, source,
		)
	}

	instance := opts.Instance
	if instance == "" {
		// Mirror Create's naming: the instance derives from the branch or
		// workspace, sanitised for use as a Compose project name.
		instance = BaseName(discovered.name)
	}

	// CreatedAt for an adopted record is its registration time. The checkout's
	// own VCS creation time is not known and is not inferred.
	return &Result{
		Path:         root,
		Branch:       discovered.name,
		SourcePath:   source,
		SourceBranch: sourceBranch,
		Instance:     instance,
		CreatedAt:    time.Now().UTC(),
	}, nil
}

// ValidateAdoptedSource confirms that a recorded source still manages the
// checkout at path. Discovery is read-only and uses the checkout's own
// repository, not the registry or a potentially stale registration in the
// recorded source's repository.
func ValidateAdoptedSource(ctx context.Context, path, source string) error {
	b, err := detectBackend(ctx, path)
	if err != nil {
		return err
	}

	discovered, err := b.linkedCheckout(ctx, path)
	if err != nil {
		return err
	}

	if resolveSymlinks(discovered.root) == resolveSymlinks(discovered.source) ||
		resolveSymlinks(discovered.source) != resolveSymlinks(source) {
		return fmt.Errorf("%s is no longer a linked checkout of %s; refresh or remove its registry entry", path, source)
	}

	return nil
}

// requireXenForoCheckout reports whether root is a XenForo checkout.
//
// A missing or wrong-type marker is an explanatory ErrNotXenForo; a permission
// or I/O failure keeps its own cause, because it says nothing about whether
// the marker exists.
func requireXenForoCheckout(root string) error {
	xfPath := filepath.Join(root, "src", "XF.php")

	info, err := os.Stat(xfPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: src/XF.php not found in %s", ErrNotXenForo, root)
		}

		return fmt.Errorf("failed to inspect %s: %w", xfPath, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrNotXenForo, xfPath)
	}

	return nil
}
