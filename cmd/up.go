package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
)

var upCmd = &cobra.Command{
	Use:   "up [path]",
	Short: "Start the Docker environment",
	Long: `Start the Docker environment for a XenForo installation.

If no path is provided, the current directory will be searched for a XenForo installation.

Examples:
  # Start in current directory (auto-detect)
  xf up

  # Start specific directory
  xf up ./my-project

  # Start in foreground (not detached)
  xf up --no-detach`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUp,
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

	if err := dockercompose.CheckDockerRunning(); err != nil {
		return err
	}

	if err := dockercompose.CheckDockerComposeAvailable(); err != nil {
		return err
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		return err
	}

	ui.PrintInfo(fmt.Sprintf("Starting Docker environment: %s", runner.Instance()))
	ui.PrintDetail(fmt.Sprintf("Directory: %s", ui.Path.Render(xfDir)))

	detach := flagUpDetach
	if cmd.Flags().Changed("no-detach") {
		detach = false
	}

	if err := runner.Up(detach); err != nil {
		return err
	}

	ui.PrintSuccess("Docker environment started")

	url, err := runner.GetURL()
	if err == nil && url != "" {
		fmt.Println()
		fmt.Printf("%s Access your site at: %s\n", ui.StatusIcon("success"), ui.URL.Render(url))
	}

	return nil
}

// getXenForoDir gets the XenForo directory from args or auto-detects.
func getXenForoDir(args []string) (string, error) {
	if len(args) > 0 {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return "", clierrors.Wrap(clierrors.CodeInvalidInput, "invalid path", err)
		}

		xfPath := filepath.Join(absPath, "src", "XF.php")
		if _, err := os.Stat(xfPath); os.IsNotExist(err) {
			return "", clierrors.Newf(clierrors.CodeInvalidInput, "not a XenForo directory: %s", absPath)
		}

		return absPath, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to get working directory", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return "", err
	}

	return xfDir, nil
}
