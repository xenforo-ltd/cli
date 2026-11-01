package worktree

import (
	"fmt"
	"strings"

	"github.com/xenforo-ltd/cli/internal/clierror"
)

// resolveBranchArg returns the branch to create.
func resolveBranchArg(args []string) (string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", fmt.Errorf(
			"a branch name is required, for example %q: %w",
			"xf worktree create dev/24x/feature", clierror.ErrInvalidInput,
		)
	}

	return args[0], nil
}
