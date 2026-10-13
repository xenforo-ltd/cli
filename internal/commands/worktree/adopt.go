package worktree

import (
	"context"
	"fmt"
	"strings"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// AdoptWorktree registers the linked checkout containing path without modifying
// it. It warns when the checkout shares the source's instance, because removal
// would refuse to tear that environment down.
func AdoptWorktree(ctx context.Context, path string, opts AdoptOptions) error {
	if strings.TrimSpace(path) == "" {
		return clierror.Usage(fmt.Errorf("a path is required, for example %q", "xf worktree adopt ../feature"))
	}

	result, err := adopt(ctx, path, opts.Instance)
	if err != nil {
		return err
	}

	reg, err := newRegistry()
	if err != nil {
		return fmt.Errorf("failed to register the worktree: %w", err)
	}

	err = reg.add(entry{
		SourcePath:   result.sourcePath,
		SourceBranch: result.sourceBranch,
		WorktreePath: result.path,
		Branch:       result.branch,
		Instance:     result.instance,
		CreatedAt:    result.createdAt,
	})
	if err != nil {
		return fmt.Errorf("failed to register the worktree: %w", err)
	}

	if opts.JSON {
		return printJSON(worktreeOutput{
			Path:         result.path,
			Branch:       result.branch,
			SourcePath:   result.sourcePath,
			SourceBranch: result.sourceBranch,
			Instance:     result.instance,
			CreatedAt:    result.createdAt,
		})
	}

	if shared, err := sharedEnvironment(result.sourcePath, result.path); err == nil && shared != "" {
		ui.PrintWarning(fmt.Sprintf(
			"This checkout uses the source environment's instance %q, so 'xf worktree remove' will refuse to tear down its environment. Use --keep-containers, or give the checkout its own XF_INSTANCE.",
			shared,
		))
	}

	ui.SuccessBox("Registered worktree", []ui.KVPair{
		ui.KV("Path", ui.Path.Render(ui.ShortHome(result.path))),
		ui.KV("Branch", result.branch),
		ui.KV("Source", ui.Path.Render(ui.ShortHome(result.sourcePath))),
		ui.KV("Instance", result.instance),
	})

	return nil
}
