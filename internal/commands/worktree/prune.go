package worktree

import (
	"fmt"
	"time"

	"github.com/xenforo-ltd/cli/internal/ui"
)

func Prune() error {
	registry, err := newRegistry()
	if err != nil {
		return err
	}

	pruned, err := registry.prune(time.Now())
	if err != nil {
		return registryReadError(err)
	}

	if pruned == 0 {
		ui.PrintEmpty("No stale worktree entries found")

		return nil
	}

	ui.PrintSuccess(fmt.Sprintf("Pruned %s", ui.Plural(pruned, "stale worktree entry", "stale worktree entries")))

	return nil
}
