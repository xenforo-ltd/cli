package build

import (
	"context"
	"os"

	"github.com/xenforo-ltd/cli/internal/build"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// JavaScript forwards a build or watch operation to the build package.
func JavaScript(ctx context.Context, args []string, watch bool) error {
	xfDir, addOnID, err := getBuildTarget(args)
	if err != nil {
		return err
	}

	if watch {
		ui.PrintInfo("Watching JavaScript for " + addOnID)
		ui.Println()
		spinner := ui.NewSpinner("Rebuilding JavaScript")
		defer spinner.Stop()
		return build.WatchJavaScript(ctx, xfDir, addOnID, build.WatchOptions{
			Stderr: os.Stderr,
			OnBuild: func() func(error) {
				spinner.Start()
				return func(err error) {
					if err != nil {
						spinner.Stop()
					} else {
						spinner.StopWithMessage("success", "JavaScript rebuilt")
					}
				}
			},
		})
	}

	spinner := ui.NewSpinner("Building JavaScript")
	defer spinner.Stop()
	spinner.Start()
	if err := build.BuildJavaScript(ctx, xfDir, addOnID); err != nil {
		return err
	}

	spinner.StopWithMessage("success", "Built JavaScript for "+addOnID)
	return nil
}
