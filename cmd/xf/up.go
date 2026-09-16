package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
)

var upCmd = &cobra.Command{
	Use:   "up [path]",
	Short: "Start the Docker environment",
	Long: `Start the Docker environment for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.`,
	Example: `  # Start in current directory (auto-detect)
  xf up

  # Start specific directory
  xf up ./my-project

  # Start in foreground (not detached)
  xf up --no-detach`,
	Args:    cobra.MaximumNArgs(1),
	GroupID: "env",
	RunE:    runUp,
}

var flagUpDetach bool

func init() {
	upCmd.Flags().BoolVar(&flagUpDetach, "detach", true, "Run containers in the background")
	upCmd.Flags().Bool("no-detach", false, "Run containers in the foreground")

	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	xfDir, err := getXenForoDir(args)
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	if err := dockercompose.CheckDockerRunning(ctx); err != nil {
		return fmt.Errorf("failed to verify Docker is running: %w", err)
	}

	if err := dockercompose.CheckDockerComposeAvailable(ctx); err != nil {
		return fmt.Errorf("failed to verify Docker Compose is available: %w", err)
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	ui.PrintInfo("Starting Docker environment " + ui.Bold.Render(runner.Instance()) + " " + ui.Muted.Render("("+ui.ShortHome(xfDir)+")"))

	detach := flagUpDetach
	if cmd.Flags().Changed("no-detach") {
		detach = false
	}

	if err := runner.Up(ctx, detach); err != nil {
		return fmt.Errorf("failed to start Docker environment: %w", err)
	}

	details := []ui.KVPair{}
	if url, err := runner.GetURL(ctx); err == nil {
		details = append(details, ui.KV("URL", ui.URL.Render(url)))
	} else {
		ui.PrintWarning("Could not determine the site URL")
	}
	ui.Println()
	ui.SuccessBox("Docker environment started", details)

	return nil
}

// getXenForoDir gets the XenForo directory from args or auto-detects.
func getXenForoDir(args []string) (string, error) {
	if len(args) > 0 {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid path: %w", err)
		}

		xfPath := filepath.Join(absPath, "src", "XF.php")
		if _, err := os.Stat(xfPath); err != nil {
			if os.IsNotExist(err) {
				return "", markAs(os.ErrNotExist, "not a XenForo installation: %s (no src/XF.php found)", absPath)
			}

			return "", fmt.Errorf("cannot access %s: %w", absPath, err)
		}

		return absPath, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to find XenForo directory: %w", err)
	}

	return xfDir, nil
}
