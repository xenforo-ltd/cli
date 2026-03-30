package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var execCmd = &cobra.Command{
	Use:   "exec [path] <service> <command> [args...]",
	Short: "Execute a command in a running container",
	Long: `Execute a command in a running Docker container.

If no path is provided, the current directory will be searched for a XenForo installation.

Examples:
  # Run a command in the xf container
  xf exec xf ls -la

  # Run a command in specific directory
  xf exec ./my-project xf bash

  # Run arbitrary docker compose command
  xf compose -- exec xf mysql -u root`,
	Args: cobra.MinimumNArgs(1),
	RunE: runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)
}

func runExec(cmd *cobra.Command, args []string) error {
	xfDir, execArgs, err := resolveXenForoDirAndArgs(args)
	if err != nil {
		return err
	}

	if err := validateExecInvocation(execArgs); err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	service := execArgs[0]
	cmdArgs := execArgs[1:]

	ui.PrintInfo(fmt.Sprintf("Executing in %s: %s", service, strings.Join(cmdArgs, " ")))

	if err := runner.Exec(cmd.Context(), service, cmdArgs...); err != nil {
		return fmt.Errorf("failed to execute command in service %s: %w", service, err)
	}

	return nil
}

func resolveXenForoDirAndArgs(args []string) (string, []string, error) {
	if len(args) > 0 {
		potentialPath := args[0]
		if dir, err := getXenForoDir([]string{potentialPath}); err == nil {
			return dir, args[1:], nil
		}
	}

	xfDir, err := getXenForoDir(nil)
	if err != nil {
		return "", nil, err
	}

	return xfDir, args, nil
}

func validateExecInvocation(execArgs []string) error {
	if len(execArgs) < 2 {
		return fmt.Errorf("exec requires <service> <command> [args...]: %w", ErrInvalidInput)
	}

	return nil
}
