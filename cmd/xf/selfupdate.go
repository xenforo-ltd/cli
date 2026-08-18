package main

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
Use --check-only to just check if an update is available without installing.`,
	Example: `  # Check for and install updates
  xf self-update

  # Just check for updates without installing
  xf self-update --check-only`,
	Args:    cobra.NoArgs,
	GroupID: "maint",
	RunE:    runSelfUpdate,
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
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	spinner.StopWithMessage("info", "Current version "+ui.Version.Render(info.CurrentVersion)+", latest "+ui.Version.Render(info.LatestVersion))

	if !info.HasUpdate {
		ui.PrintSuccess("Already on the latest version")
		return nil
	}

	ui.PrintInfo("Update available: " + ui.Version.Render(info.LatestVersion))

	if info.ReleaseURL != "" {
		ui.PrintKeyValuePadded([]ui.KVPair{ui.KV("Release notes", ui.URL.Render(info.ReleaseURL))})
	}

	if selfUpdateCheckOnly {
		ui.PrintHint("Run " + ui.Command.Render("xf self-update") + " to install")
		return nil
	}

	ui.PrintSubstep("Downloading " + info.AssetName)

	var (
		progressBar *ui.ProgressBar
		dlSpinner   *ui.Spinner
	)

	err = updater.Update(ctx, info, func(downloaded, total int64) {
		if total > 0 {
			if dlSpinner != nil {
				dlSpinner.Stop()
				dlSpinner = nil
			}

			if progressBar == nil {
				progressBar = ui.NewProgressBar(total, info.AssetName)
			}

			progressBar.Update(downloaded)
		} else if dlSpinner == nil {
			dlSpinner = ui.NewSpinner("Downloading " + info.AssetName)
			dlSpinner.Start()
		}
	})
	if err != nil {
		if progressBar != nil {
			progressBar.Abandon()
		}

		if dlSpinner != nil {
			dlSpinner.Stop()
		}

		return fmt.Errorf("failed to install self-update: %w", err)
	}

	if progressBar != nil {
		progressBar.Finish()
	}

	if dlSpinner != nil {
		dlSpinner.Stop()
	}

	ui.Println()
	ui.SuccessBox("Updated to "+ui.Version.Render(info.LatestVersion), []ui.KVPair{
		ui.KV("Previous version", info.CurrentVersion),
		ui.KV("Current version", info.LatestVersion),
	})

	return nil
}
