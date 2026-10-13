package worktree

import (
	"errors"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// registryReadError explains how to recover from an unreadable registry.
//
// The registry is only a record of what xf created or adopted, so deleting it
// costs nothing but the cross-project listing and the adoptions, which can be
// repeated: the worktrees themselves are still on disk and still known to the
// version control system. Saying so turns a dead end into one command.
func registryReadError(err error) error {
	if !errors.Is(err, errRegistryCorrupt) {
		return err
	}

	path, pathErr := registryPath()
	if pathErr != nil {
		return err
	}

	return clierror.WithHint(err, "Checkout files and version-control registrations are unaffected: delete "+
		ui.Path.Render(ui.ShortHome(path))+" to start a new record, then adopt any linked checkouts again")
}

// warnRegistryUpdateFailed reports a registry write failure without printing
// the underlying error: the operation it was recording already succeeded, so
// this is advisory, not a failure report.
func warnRegistryUpdateFailed() {
	ui.PrintWarning("Could not update the worktree registry")
}
