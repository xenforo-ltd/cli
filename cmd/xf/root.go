package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/version"
	"github.com/xenforo-ltd/cli/internal/xf"
)

var configFile string

var rootCmd = &cobra.Command{
	Use:     "xf",
	Version: version.Version,
	// Required by the passthrough commands, which set DisableFlagParsing so that
	// everything after them reaches the target tool. Without TraverseChildren,
	// cobra defers parsing the root's persistent flags to the subcommand, which
	// then forwards them instead: even `xf --verbose php script.php` would be
	// swallowed, leaving no way to set xf's own flags on those commands.
	TraverseChildren: true,
	Short:            "Provision and manage XenForo development environments",
	Long: `The XenForo CLI is a command-line tool for provisioning and managing
XenForo development environments using Docker.

It handles OAuth authentication, downloads XenForo packages, manages
caching, and orchestrates Docker-based development environments.

Get started by authenticating:
  xf auth login

Then initialize a new project:
  xf init ./my-project

Run XenForo commands directly (from a XenForo directory):
  xf list
  xf xf-dev:import
`,
}

// usageError marks an error as caused by incorrect invocation (bad arguments or
// flags), for which printing usage is genuinely helpful.
type usageError struct {
	err error
}

func newUsageError(err error) error {
	return &usageError{err: err}
}

func (e *usageError) Error() string { return e.err.Error() }

func (e *usageError) Unwrap() error { return e.err }

// configureErrorHandling makes cobra print usage only for genuine misuse.
//
// By default cobra prints the full usage block for any error returned from RunE,
// including runtime failures such as Docker being unavailable. That wrongly
// implies the command was typed incorrectly. Usage is still printed for argument
// and flag errors, where it is the helpful response.
func configureErrorHandling(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	// Execute prints errors itself; without this cobra prints them too.
	cmd.SilenceErrors = true

	// Execute may be called more than once by tests or an embedding program. Do
	// not stack another usageError wrapper on every invocation.
	if cmd.Args != nil && (cmd.Annotations == nil || cmd.Annotations[usageConfiguredAnnotation] != "true") {
		args := cmd.Args
		cmd.Args = func(c *cobra.Command, a []string) error {
			if err := args(c, a); err != nil {
				return newUsageError(err)
			}

			return nil
		}
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[usageConfiguredAnnotation] = "true"

	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return newUsageError(err)
	})

	for _, sub := range cmd.Commands() {
		configureErrorHandling(sub)
	}
}

const usageConfiguredAnnotation = "xf.xenforo.com/usage-error-handling-configured"

// Execute runs the CLI application.
func Execute(ctx context.Context) {
	configureErrorHandling(rootCmd)

	if len(os.Args) > 1 {
		firstArg := os.Args[1]

		if takesDirectXenForoRoute(firstArg) {
			if !isKnownCommand(firstArg) {
				if err := runAsXenForoCommand(ctx, os.Args[1:], exec.Command); err != nil {
					handleError(err)
					os.Exit(1)
				}

				return
			}
		}
	}

	executed, err := rootCmd.ExecuteContextC(ctx)
	if err != nil {
		handleError(err)

		var usageErr *usageError
		if errors.As(err, &usageErr) && executed != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprint(os.Stderr, executed.UsageString())
		}

		os.Exit(1)
	}
}

// takesDirectXenForoRoute reports whether a first argument is eligible to be
// forwarded straight to XenForo, bypassing cobra.
//
// Known limitation: a leading flag is not eligible, so a global flag cannot be
// combined with a direct XenForo command. `xf -v xf-dev:import` falls through to
// cobra, which resolves nothing and prints the root help without an error.
func takesDirectXenForoRoute(firstArg string) bool {
	return !strings.HasPrefix(firstArg, "-") &&
		firstArg != "help" && firstArg != "--help" && firstArg != "-h"
}

func handleError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
}

func isKnownCommand(name string) bool {
	if found, _, err := rootCmd.Find([]string{name}); err == nil && found != nil && found.Name() == name {
		return true
	}

	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			return true
		}

		if slices.Contains(cmd.Aliases, name) {
			return true
		}
	}

	return false
}

func runAsXenForoCommand(ctx context.Context, args []string, cmdFn func(string, ...string) *exec.Cmd) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return fmt.Errorf("unknown command: %s (not in a XenForo directory): %w", args[0], err)
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		if errors.Is(err, dockercompose.ErrEnvNotInitialized) {
			return runAsLocalXenForoCommand(xfDir, args, cmdFn)
		}

		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if err := runner.XFCommand(ctx, args...); err != nil {
		return fmt.Errorf("failed to run XenForo command %q: %w", args[0], err)
	}

	return nil
}

func runAsLocalXenForoCommand(xfDir string, args []string, cmdFn func(string, ...string) *exec.Cmd) error {
	cmdArgs := append([]string{"cmd.php"}, args...)
	cmd := cmdFn("php", cmdArgs...)
	cmd.Dir = xfDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("local PHP executable not found in PATH: %w", err)
		}

		return fmt.Errorf("local XenForo command failed: %w", err)
	}

	return nil
}

func init() {
	cobra.OnInitialize(func() {
		if err := config.Init(configFile); err != nil {
			if errors.As(err, &viper.ConfigFileNotFoundError{}) {
				return
			}

			cobra.CheckErr(err)
		}
	})

	rootCmd.InitDefaultCompletionCmd()

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "path to config file")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "disable interactive prompts")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")

	if err := viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		cobra.CheckErr(err)
	}

	if err := viper.BindPFlag("no_interaction", rootCmd.PersistentFlags().Lookup("no-interaction")); err != nil {
		cobra.CheckErr(err)
	}
}
