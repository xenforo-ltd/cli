package worktree

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/xenforo-ltd/cli/internal/environment"
)

// maxReserveAttempts bounds how often reserveWorktreeName retries after
// another process claims the name it chose.
const maxReserveAttempts = 10

// reserveWorktreeName chooses a name for a new worktree of branch and records
// it in the registry, returning the reserved entry.
//
// Choosing and recording are separate steps, so another xf process can claim
// the same name in between. reserve detects that under the registry lock, and
// the name is chosen again, now seeing the other process's entry.
//
// The worktree's name doubles as its instance name unless --instance gave one.
// An explicit instance is the user's choice, so a clash on it is reported
// rather than numbered around. The directory is still numbered.
func reserveWorktreeName(ctx context.Context, registry *registry, source, branch, instance string, noSetup bool) (entry, error) {
	sourceInstance, err := configuredInstance(source)
	if err != nil {
		return entry{}, err
	}

	for range maxReserveAttempts {
		if instance != "" {
			taken, err := instanceTaken(ctx, registry, sourceInstance, instance, noSetup)
			if err != nil {
				return entry{}, err
			}

			if taken {
				return entry{}, fmt.Errorf(
					"the instance name %q is already in use by another environment; choose another name with --instance",
					instance,
				)
			}
		}

		name, err := chooseName(source, branch, func(name string) (bool, error) {
			reserved, err := directoryReserved(registry, source, name)
			if err != nil || reserved {
				return reserved, err
			}

			// With --instance the name is only a directory, so it cannot
			// clash with another environment's instance.
			if instance != "" {
				return false, nil
			}

			return instanceTaken(ctx, registry, sourceInstance, name, noSetup)
		})
		if err != nil {
			return entry{}, err
		}

		reserved := entry{
			SourcePath:   source,
			WorktreePath: filepath.Join(worktreesDir(source), name),
			Branch:       branch,
			Instance:     defaultString(instance, name),
			CreatedAt:    time.Now().UTC(),
		}

		err = registry.reserve(reserved)
		if err == nil {
			return reserved, nil
		}

		if !errors.Is(err, errNameReserved) {
			return entry{}, registryReadError(err)
		}
	}

	return entry{}, fmt.Errorf("could not reserve a worktree name for %q: other xf processes kept claiming it", branch)
}

// directoryReserved reports whether a registry entry, possibly a reservation
// for a worktree still being created, holds the directory name in source's
// worktrees directory. A reservation made with --instance holds a directory
// whose name differs from its instance, and the directory does not exist until
// the creation finishes, so neither the instance check nor the filesystem
// would see it.
func directoryReserved(registry *registry, source, name string) (bool, error) {
	entries, err := registry.all()
	if err != nil {
		return false, registryReadError(err)
	}

	path := filepath.Clean(filepath.Join(worktreesDir(source), name))

	for _, entry := range entries {
		if filepath.Clean(entry.WorktreePath) == path {
			return true, nil
		}
	}

	return false, nil
}

// instanceTaken reports whether an instance name is already used by another
// environment.
//
// Docker Compose projects are global, so a project left by another checkout,
// or by a worktree removed with --keep-containers, would otherwise be shared.
// The registry covers worktrees with no project yet: those created with
// --no-setup, and those still being created by another process. The source's
// configured instance is reserved even if it has never been started.
//
// Every registry entry counts, including ones whose worktree has gone, until
// "xf worktree prune" drops them: an entry can be a reservation for a worktree
// that does not exist yet, and prune leaves those alone. A registry that
// cannot be read is an error rather than an empty one, because it is what
// keeps names unique.
//
// With --no-setup Docker need not be running, so failing to reach it is not
// an error there.
func instanceTaken(ctx context.Context, registry *registry, sourceInstance, name string, noSetup bool) (bool, error) {
	if name == sourceInstance {
		return true, nil
	}

	entries, err := registry.all()
	if err != nil {
		return false, registryReadError(err)
	}

	for _, entry := range entries {
		if entry.Instance == name {
			return true, nil
		}
	}

	exists, err := environment.ProjectExists(ctx, name)
	if err != nil {
		if noSetup {
			return false, nil
		}

		return false, err
	}

	return exists, nil
}

// configuredInstance returns the instance name the checkout at dir is
// configured to use, or an empty string when it has no environment.
func configuredInstance(dir string) (string, error) {
	runner, err := environment.NewRunner(dir)
	if err != nil {
		if errors.Is(err, environment.ErrEnvNotInitialized) {
			return "", nil
		}

		return "", fmt.Errorf("failed to read the source environment: %w", err)
	}

	return runner.Instance(), nil
}

// defaultString returns value, or fallback when value is empty.
func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}

	return fallback
}
