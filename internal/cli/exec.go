package cli

import (
	"github.com/spf13/cobra"

	commandexec "github.com/xenforo-ltd/cli/internal/commands/exec"
)

// newExecCommand builds the exec command. Arguments and flags after its name
// belong to the target command; xf flags must precede it.
func newExecCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "exec [path] <service> <command> [args...]",
		Short: "Execute a command in a running container",
		Long: `Execute a command in a running Docker container.

If no path is provided, the current directory will be searched for a XenForo installation.`,
		Example: `  # Run a command in the xf container
  xf exec xf ls -la

  # Run a command in specific directory
  xf exec ./my-project xf bash

  # Run arbitrary docker compose command
  xf compose -- exec xf mysql -u root`,
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		GroupID:            "run",
		RunE:               runExec,
	}
}

// runExec executes a command in a running container.
func runExec(cmd *cobra.Command, args []string) error {
	return commandexec.Run(cmd.Context(), args)
}
