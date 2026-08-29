package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

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
  - Network connectivity`,
	Example: `  # Run all health checks
  xf doctor`,
	Args:    cobra.NoArgs,
	GroupID: "maint",
	RunE:    runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	doc := doctor.NewDoctor()

	ui.Println(ui.Header.Render("System health check"))
	ui.Println()

	results := doc.RunAll(cmd.Context())

	var passed, warned, failed, skipped int

	for _, result := range results {
		var statusStr string

		switch result.Status {
		case doctor.StatusOK:
			statusStr = ui.StatusIcon("success")
			passed++
		case doctor.StatusWarning:
			statusStr = ui.StatusIcon("warning")
			warned++
		case doctor.StatusError:
			statusStr = ui.StatusIcon("error")
			failed++
		case doctor.StatusSkipped:
			statusStr = ui.StatusIcon("skipped")
			skipped++
		default:
			statusStr = ui.StatusIcon("pending")
		}

		line := fmt.Sprintf("%s %s", statusStr, ui.Bold.Render(result.Name))
		if result.Message != "" {
			line += "  " + ui.Dim.Render(result.Message)
		}
		ui.Println(line)

		if result.Details != "" {
			for line := range strings.SplitSeq(result.Details, "\n") {
				ui.Printf("%s%s\n", ui.Indent1, ui.Dim.Render(line))
			}
		}

		if result.Suggestion != "" {
			ui.PrintHint(result.Suggestion)
		}
	}

	total := len(results)

	// Skipped checks neither passed nor failed, so they are always reported
	// separately rather than folded into the passed count.
	skippedSuffix := ""
	if skipped > 0 {
		skippedSuffix = fmt.Sprintf(", %d skipped", skipped)
	}

	ui.Println()

	switch {
	case failed > 0:
		ui.PrintError(fmt.Sprintf("%s failed (%d passed, %d warnings%s)",
			ui.Plural(failed, "check", "checks"), passed, warned, skippedSuffix))

		return newExitCodeError(1)
	case warned > 0:
		ui.PrintWarning(fmt.Sprintf("%s passed, %s%s",
			ui.Plural(passed, "check", "checks"),
			ui.Plural(warned, "check has a warning", "checks have warnings"),
			skippedSuffix))
	case skipped > 0:
		ui.PrintSuccess(fmt.Sprintf("%s passed, %d skipped", ui.Plural(passed, "check", "checks"), skipped))
	default:
		ui.PrintSuccess(fmt.Sprintf("All %d checks passed", total))
	}

	return nil
}
