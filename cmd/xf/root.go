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
	"github.com/xenforo-ltd/cli/internal/xf"
)

var configFile string

var rootCmd = &cobra.Command{
	Use:   "xf",
	Short: "Provision and manage XenForo development environments",
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

// Execute runs the CLI application.
func Execute(ctx context.Context) {
	if len(os.Args) > 1 {
		firstArg := os.Args[1]

		if !strings.HasPrefix(firstArg, "-") && firstArg != "help" && firstArg != "--help" && firstArg != "-h" {
			if !isKnownCommand(firstArg) {
				if err := runAsXenForoCommand(ctx, os.Args[1:], exec.Command); err != nil {
					handleError(err)
					os.Exit(1)
				}

				return
			}
		}
	}

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		handleError(err)
		os.Exit(1)
	}
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
