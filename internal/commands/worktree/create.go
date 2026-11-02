package worktree

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/setup"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func CreateWorktree(ctx context.Context, args []string, opts CreateOptions) error {
	// JSON must be the only thing on stdout, but setup writes progress there.
	// Requiring --no-setup keeps the stream machine-readable. This runs before
	// any path resolution or Git state changes so a rejected invocation leaves
	// nothing behind.
	if opts.JSON && !opts.NoSetup {
		return clierror.Usage(fmt.Errorf(
			"--json requires --no-setup: setup progress is written to stdout and would corrupt the JSON document",
		))
	}

	branch, err := resolveBranchArg(args)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := sourceCheckout(ctx, cwd)
	if err != nil {
		return err
	}

	reg, err := newRegistry()
	if err != nil {
		return err
	}

	reserved, err := reserveWorktreeName(ctx, reg, source, branch, opts.Instance, opts.NoSetup)
	if err != nil {
		return err
	}

	result, err := create(ctx, backendOptions{
		sourcePath: source,
		branch:     branch,
		name:       filepath.Base(reserved.WorktreePath),
		base:       opts.Base,
		instance:   reserved.Instance,
	})
	if err != nil {
		// Release the name so a retry can use it, unless creation left the
		// directory behind. chooseName skips that directory for this source,
		// but only the reservation keeps another checkout from reusing the
		// instance name.
		if _, statErr := os.Lstat(reserved.WorktreePath); errors.Is(statErr, fs.ErrNotExist) {
			if removeErr := reg.remove(reserved.WorktreePath); removeErr != nil {
				warnRegistryUpdateFailed()
			}
		}

		return err
	}

	record := entry{
		SourcePath:   result.sourcePath,
		SourceBranch: result.sourceBranch,
		WorktreePath: result.path,
		Branch:       result.branch,
		Instance:     result.instance,
		CreatedAt:    result.createdAt,
	}

	if err := reg.add(record); err != nil {
		// The worktree exists and is usable; a registry failure must not
		// present itself as a failed creation.
		warnRegistryUpdateFailed()
	}

	if !opts.JSON {
		ui.SuccessBox("Created worktree", []ui.KVPair{
			ui.KV("Path", ui.Path.Render(ui.ShortHome(result.path))),
			ui.KV("Branch", result.branch),
			ui.KV("Base branch", result.sourceBranch),
			ui.KV("Instance", result.instance),
		})
	}

	// Cloning imports a database that is already installed, so xf:install must
	// not run over it: it would wipe the data that was just copied.
	//
	// Cloning needs running containers, so --no-up rules it out. Treating the
	// worktree as cloning anyway would suppress xf:install as well, leaving it
	// with neither an imported database nor an installed one.
	cloning := false

	if !opts.Fresh && !opts.NoUp {
		installed, err := sourceIsInstalled(result.sourcePath)
		if err != nil {
			return err
		}

		cloning = installed
	}

	// --no-up on an installed source leaves the worktree configured but with
	// neither a cloned database nor an installed one, and nothing about a
	// successful "create" says so. A later "xf up" would then present an empty
	// board with no explanation, so name the gap and the way to close it.
	if opts.NoUp && !opts.Fresh && !opts.NoSetup && !opts.JSON {
		installed, err := sourceIsInstalled(result.sourcePath)
		if err != nil {
			return err
		}

		if installed {
			ui.PrintWarning("--no-up skips cloning the source environment, so this worktree has no database yet. " +
				"Run xf worktree create without --no-up to clone it, or xf up and then install manually.")
		}
	}

	if !opts.NoSetup {
		if err := setup.Existing(ctx, worktreeInitOptions(result, cloning, opts)); err != nil {
			return fmt.Errorf("worktree created at %s, but setting up its environment failed: %w", result.path, err)
		}

		if cloning {
			if err := cloneEnvironment(ctx, result.sourcePath, result.path); err != nil {
				return fmt.Errorf("worktree created at %s, but cloning the environment failed: %w", result.path, err)
			}

			record.Cloned = true

			if err := reg.add(record); err != nil {
				warnRegistryUpdateFailed()
			}
		}

		// Only a fresh install has credentials worth reporting. A cloned
		// worktree keeps the source's own logins, so printing the defaults
		// would be wrong.
		if !cloning && !opts.JSON && !opts.NoUp {
			ui.Println()
			ui.InfoBox("Fresh install credentials", []ui.KVPair{
				ui.KV("Admin user", defaultString(opts.AdminUser, defaultWorktreeAdminUser)),
				ui.KV("Admin password", defaultString(opts.AdminPassword, defaultWorktreeAdminPassword)),
			})
		}
	}

	if opts.JSON {
		return printJSON(worktreeOutput{
			Path:         result.path,
			Branch:       result.branch,
			SourcePath:   result.sourcePath,
			SourceBranch: result.sourceBranch,
			Instance:     result.instance,
			Cloned:       record.Cloned,
			CreatedAt:    result.createdAt,
		})
	}

	return nil
}
