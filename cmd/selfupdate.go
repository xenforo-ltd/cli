package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/selfupdate"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var selfUpdateCheckOnly bool

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update XenForo CLI to the latest version",
	Long: `Check for and install updates to the xf tool.

By default, this command will check for updates and install them automatically.
Use --check-only to just check if an update is available without installing.

Examples:
  # Check for and install updates
  xf self-update

  # Just check for updates without installing
  xf self-update --check-only`,
	RunE: runSelfUpdate,
}

func init() {
	rootCmd.AddCommand(selfUpdateCmd)

	selfUpdateCmd.Flags().BoolVar(&selfUpdateCheckOnly, "check-only", false,
		"only check for updates, don't install")
}

func runSelfUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	updater := selfupdate.NewUpdater()

	spinner := ui.NewSpinner("Checking for updates...")
	spinner.Start()

	info, err := updater.CheckForUpdate(ctx)
	if err != nil {
		spinner.StopWithMessage("error", "Failed to check for updates")
		return err
	}

	spinner.Stop()

	ui.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("Current version", ui.Version.Render(info.CurrentVersion)),
		ui.KV("Latest version", ui.Version.Render(info.LatestVersion)),
	})
	ui.Println()

	if !info.HasUpdate {
		ui.PrintSuccess("You are already running the latest version.")
		return nil
	}

	ui.PrintInfo("A new version is available!")

	if info.ReleaseURL != "" {
		ui.Printf("Release notes: %s\n", ui.URL.Render(info.ReleaseURL))
	}

	ui.Println()

	if selfUpdateCheckOnly {
		ui.PrintWarning(fmt.Sprintf("Run '%s' to install the update.", ui.Command.Render("xf self-update")))
		return nil
	}

	ui.Printf("Downloading %s...\n", info.AssetName)

	var progressBar *ui.ProgressBar

	err = updater.Update(ctx, info, func(downloaded, total int64) {
		if total > 0 {
			if progressBar == nil {
				progressBar = ui.NewProgressBar(total, "")
			}

			progressBar.Update(downloaded)
		}
	})
	if err != nil {
		ui.Println()
		return err
	}

	if progressBar != nil {
		progressBar.Finish()
	}

	ui.Println()
	ui.PrintSuccess("Update successful!")
	ui.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("Previous", ui.Version.Render(info.CurrentVersion)),
		ui.KV("Current", ui.Version.Render(info.LatestVersion)),
	})
	ui.Println()
	ui.Printf("Run '%s' to verify the update.\n", ui.Command.Render("xf version"))

	return nil
}
