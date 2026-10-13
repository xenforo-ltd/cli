package worktree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/environment"
)

var (
	// errInvalidPath indicates a path that cannot be adopted.
	errInvalidPath = errors.New("invalid worktree path")

	// errNotAWorktree indicates a path that is not a registered checkout.
	errNotAWorktree = errors.New("not a registered worktree or workspace")

	// errNoBranch indicates a Git checkout with a detached HEAD.
	errNoBranch = errors.New("checkout has no branch")
)

// adopt describes the linked checkout containing path. It does not modify the
// checkout.
//
// The instance is instance if non-empty, else XF_INSTANCE from the checkout's
// .env file, else a name derived from the branch.
func adopt(ctx context.Context, path, instance string) (*backendResult, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", path, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s does not exist", errInvalidPath, path)
		}

		return nil, fmt.Errorf("failed to inspect %s: %w", path, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", errInvalidPath, path)
	}

	dir, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %w", path, err)
	}

	b, err := detectBackend(ctx, dir)
	if err != nil {
		return nil, err
	}

	discovered, err := b.linkedCheckout(ctx, dir)
	if err != nil {
		return nil, err
	}

	root := resolveSymlinks(discovered.root)
	source := resolveSymlinks(discovered.source)

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

	if instance == "" {
		instance, err = environment.ReadInstanceName(root)
		if err != nil {
			return nil, err
		}
	}

	return &backendResult{
		path:         root,
		branch:       discovered.name,
		sourcePath:   source,
		sourceBranch: sourceBranch,
		instance:     defaultString(instance, baseName(discovered.name)),
		createdAt:    time.Now().UTC(),
	}, nil
}

// validateAdoptedSource returns an error unless path is a linked checkout of
// source.
func validateAdoptedSource(ctx context.Context, path, source string) error {
	b, err := detectBackend(ctx, path)
	if err != nil {
		return err
	}

	discovered, err := b.linkedCheckout(ctx, path)
	if err != nil {
		return err
	}

	discoveredSource := resolveSymlinks(discovered.source)

	if resolveSymlinks(discovered.root) == discoveredSource || discoveredSource != resolveSymlinks(source) {
		return fmt.Errorf("%s is no longer a linked checkout of %s; refresh or remove its registry entry", path, source)
	}

	return nil
}
