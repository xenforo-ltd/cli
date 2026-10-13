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

// CreateWorktree reserves a name, creates a checkout, and optionally sets up
// or clones its environment. JSON requires no setup so stdout remains a single
// document; this is validated before mutation. A failed creation releases its
// reservation only if no directory remains, protecting its instance name.
// Registry update failures after creation are warnings, not creation failures.
// Cloning skips installation to protect imported data and requires containers;
// no-up skips cloning and warns when an installed source cannot be copied.
// Fresh-install credentials are not reported for a clone, which retains the
// source's logins. Setup or cloning errors leave the created worktree in place.
func CreateWorktree(ctx context.Context, args []string, opts CreateOptions) error {
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

	source, err := managementSource(ctx, cwd)
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

	cloning := false

	if !opts.Fresh && !opts.NoUp {
		installed, err := sourceIsInstalled(result.sourcePath)
		if err != nil {
			return err
		}

		cloning = installed
	}

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
