package cmd

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/doctor"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and dependencies",
	Long: `Run diagnostic checks to verify that your system is properly configured
for XenForo CLI.

This command checks:
  - System keychain availability
  - Authentication status
  - Git installation
  - Docker installation and daemon status
  - Cache directory permissions
  - Disk space availability
  - Network connectivity

Examples:
  # Run all health checks
  xf doctor`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	doc := doctor.NewDoctor()

	ui.Println(ui.Bold.Render("System Health Check"))
	ui.Println()

	results := doc.RunAll(ctx)

	for _, result := range results {
		var statusStr string

		switch result.Status {
		case doctor.StatusOK:
			statusStr = ui.StatusIcon("success")
		case doctor.StatusWarning:
			statusStr = ui.StatusIcon("warning")
		case doctor.StatusError:
			statusStr = ui.StatusIcon("error")
		default:
			statusStr = ui.StatusIcon("skipped")
		}

		switch {
		case result.Message != "" && result.Status == doctor.StatusOK:
			ui.Printf("%s %s (%s)\n", statusStr, ui.Bold.Render(result.Name), ui.Dim.Render(result.Message))
		case result.Message != "":
			ui.Printf("%s %s\n", statusStr, ui.Bold.Render(result.Name))
			ui.Printf("%s%s\n", ui.Indent2, result.Message)
		default:
			ui.Printf("%s %s\n", statusStr, ui.Bold.Render(result.Name))
		}

		if result.Details != "" {
			for line := range strings.SplitSeq(result.Details, "\n") {
				ui.Printf("%s%s\n", ui.Indent2, ui.Dim.Render(line))
			}
		}

		if result.Suggestion != "" {
			ui.Printf("%s%s %s\n", ui.Indent2, ui.Dim.Render(ui.SymbolArrow), ui.Info.Render(result.Suggestion))
		}
	}

	ui.Println()

	switch {
	case doc.HasErrors():
		ui.PrintError("Some checks failed")
		return clierrors.New(clierrors.CodeInternal, "health check failed")
	case doc.HasWarnings():
		ui.PrintWarning("Some checks have warnings")
	default:
		ui.PrintSuccess("All checks passed")
	}

	return nil
}
