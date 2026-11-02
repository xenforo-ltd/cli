package selfupdate

import (
	"context"
	"fmt"

	"github.com/xenforo-ltd/cli/internal/ui"
)

func Run(ctx context.Context, checkOnly bool) error {
	u := newUpdater()

	spinner := ui.NewSpinner("Checking for updates...")
	spinner.Start()

	info, err := u.checkForUpdate(ctx)
	if err != nil {
		spinner.StopWithMessage("error", "Failed to check for updates")
		return fmt.Errorf("failed to check for updates: %w", err)
	}

	spinner.StopWithMessage("info", "Current version "+ui.Version.Render(info.currentVersion)+", latest "+ui.Version.Render(info.latestVersion))

	if !info.hasUpdate {
		ui.PrintSuccess("Already on the latest version")
		return nil
	}

	ui.PrintInfo("Update available: " + ui.Version.Render(info.latestVersion))

	if info.releaseURL != "" {
		ui.PrintKeyValuePadded([]ui.KVPair{ui.KV("Release notes", ui.URL.Render(info.releaseURL))})
	}

	if checkOnly {
		ui.PrintHint("Run " + ui.Command.Render("xf self-update") + " to install")
		return nil
	}

	ui.PrintSubstep("Downloading " + info.assetName)

	var (
		progressBar *ui.ProgressBar
		dlSpinner   *ui.Spinner
	)

	stopSpinner := func() {
		if dlSpinner != nil {
			dlSpinner.Stop()
			dlSpinner = nil
		}
	}

	err = u.update(ctx, info, func(downloaded, total int64) {
		if total > 0 {
			stopSpinner()

			if progressBar == nil {
				progressBar = ui.NewProgressBar(total, info.assetName)
			}

			progressBar.Update(downloaded)
		} else if dlSpinner == nil {
			dlSpinner = ui.NewSpinner("Downloading " + info.assetName)
			dlSpinner.Start()
		}
	})

	if err != nil {
		if progressBar != nil {
			progressBar.Abandon()
		}

		stopSpinner()

		return fmt.Errorf("failed to install self-update: %w", err)
	}

	if progressBar != nil {
		progressBar.Finish()
	}

	stopSpinner()

	ui.Println()
	ui.SuccessBox("Updated to "+ui.Version.Render(info.latestVersion), []ui.KVPair{
		ui.KV("Previous version", info.currentVersion),
		ui.KV("Current version", info.latestVersion),
	})

	return nil
}
