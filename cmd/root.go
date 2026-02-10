package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"xf/internal/config"
	"xf/internal/dockercompose"
	clierrors "xf/internal/errors"
)

var (
	flagNonInteractive bool
	flagVerbose        bool
	execCommand        = exec.Command
)

var rootCmd = &cobra.Command{
	Use:   "xf",
	Short: "Provision and manage XenForo development environments",
	Long: `xf is a command-line tool for provisioning and managing
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		config.SetFlags(config.GlobalFlags{
			NonInteractive: flagNonInteractive,
			Verbose:        flagVerbose,
		})

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if len(os.Args) > 1 {
		firstArg := os.Args[1]

		if !strings.HasPrefix(firstArg, "-") && firstArg != "help" && firstArg != "--help" && firstArg != "-h" {
			if !isKnownCommand(firstArg) {
				if err := runAsXenForoCommand(os.Args[1:]); err != nil {
					handleError(err)
					os.Exit(1)
				}
				return
			}
		}
	}

	if err := rootCmd.Execute(); err != nil {
		handleError(err)
		os.Exit(1)
	}
}

// isKnownCommand checks if a command is registered with Cobra
func isKnownCommand(name string) bool {
	if found, _, err := rootCmd.Find([]string{name}); err == nil && found != nil && found.Name() == name {
		return true
	}

	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			return true
		}
		for _, alias := range cmd.Aliases {
			if alias == name {
				return true
			}
		}
	}
	return false
}

// runAsXenForoCommand attempts to run the arguments as a XenForo CLI command
func runAsXenForoCommand(args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return clierrors.New(clierrors.CodeInvalidInput, "failed to get current directory")
	}

	xfDir, err := findXenForoDir(cwd)
	if err != nil {
		return clierrors.Newf(clierrors.CodeInvalidInput, "unknown command: %s (not in a XenForo directory)", args[0])
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		if clierrors.Is(err, clierrors.CodeDockerEnvNotInitialized) {
			return runAsLocalXenForoCommand(xfDir, args)
		}
		return err
	}

	return runner.XFCommand(args...)
}

func runAsLocalXenForoCommand(xfDir string, args []string) error {
	cmdArgs := append([]string{"cmd.php"}, args...)
	cmd := execCommand("php", cmdArgs...)
	cmd.Dir = xfDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return clierrors.New(clierrors.CodeInvalidInput, "local PHP executable not found in PATH")
		}
		return fmt.Errorf("local XenForo command failed: %w", err)
	}

	return nil
}


func findXenForoDir(startDir string) (string, error) {
	dir := filepath.Clean(startDir)
	for {
		xfPath := filepath.Join(dir, "src", "XF.php")
		if _, err := os.Stat(xfPath); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if xfDir := os.Getenv("XF_DIR"); xfDir != "" {
		if _, err := os.Stat(filepath.Join(xfDir, "src", "XF.php")); err == nil {
			return xfDir, nil
		}
	}

	return "", clierrors.New(clierrors.CodeInvalidInput, "not in a XenForo directory")
}

func init() {
	rootCmd.InitDefaultCompletionCmd()

	rootCmd.PersistentFlags().BoolVar(&flagNonInteractive, "non-interactive", false,
		"disable interactive prompts (for CI/automation)")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false,
		"enable verbose output")
}

func handleError(err error) {
	var cliErr *clierrors.CLIError
	if errors.As(err, &cliErr) {
		if flagVerbose {
			fmt.Fprintf(os.Stderr, "Error: %s\n", cliErr.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Error [%s]: %s\n", cliErr.Code, cliErr.Message)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
	}
}
