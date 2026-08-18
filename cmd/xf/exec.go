package main

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
)

var execCmd = &cobra.Command{
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
	// Everything after this command belongs to the target tool, including
	// flags. xf's own flags must be given before the command name.
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(1),
	GroupID:            "run",
	RunE:               runExec,
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

	if err := runner.Exec(cmd.Context(), service, cmdArgs...); err != nil {
		return passthroughError(err, "failed to execute command in service "+service)
	}

	return nil
}

// passthroughError converts a child process's non-zero exit into a bare
// exit-code error (the child already printed its own failure); other errors
// are wrapped with context.
func passthroughError(err error, context string) error {
	if err == nil {
		return nil
	}

	var ee *exec.ExitError
	if errors.As(err, &ee) {
		// A process killed by a signal reports -1, which is not a status any
		// caller can exit with. Those failures are reported normally instead.
		if code := ee.ExitCode(); code >= 0 {
			return newExitCodeError(code)
		}
	}

	return fmt.Errorf("%s: %w", context, err)
}

func resolveXenForoDirAndArgs(args []string) (string, []string, error) {
	if len(args) > 0 {
		potentialPath := args[0]
		if dir, err := getXenForoDir([]string{potentialPath}); err == nil {
			return dir, stripFlagSeparator(args[1:]), nil
		}
	}

	xfDir, err := getXenForoDir(nil)
	if err != nil {
		return "", nil, err
	}

	return xfDir, stripFlagSeparator(args), nil
}

func stripFlagSeparator(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}

	return args
}

func validateExecInvocation(execArgs []string) error {
	if len(execArgs) < 2 {
		return newUsageError(fmt.Errorf("exec requires <service> <command> [args...]: %w", ErrInvalidInput))
	}

	return nil
}
