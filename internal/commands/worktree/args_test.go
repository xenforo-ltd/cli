package worktree

import (
	"errors"
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierror"
)

func TestWorktreeCreateAcceptsAnyBranchName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"dev/24x/feature", "feature", "list", "help", "remove"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveBranchArg([]string{name})
			if err != nil {
				t.Errorf("resolveBranchArg(%q) returned %v, want it accepted", name, err)
			}
			if got != name {
				t.Errorf("resolveBranchArg(%q) = %q", name, got)
			}
		})
	}
}

func TestWorktreeCreateRequiresABranch(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {}, {""}, {"   "}} {
		if _, err := resolveBranchArg(args); !errors.Is(err, clierror.ErrInvalidInput) {
			t.Errorf("resolveBranchArg(%v) = %v, want a rejection", args, err)
		}
	}
}
