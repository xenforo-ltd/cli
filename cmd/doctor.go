package cmd

import (
	"context"
	"fmt"
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

	fmt.Println(ui.Bold.Render("System Health Check"))
	fmt.Println()

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

		if result.Message != "" && result.Status == doctor.StatusOK {
			fmt.Printf("%s %s (%s)\n", statusStr, ui.Bold.Render(result.Name), ui.Dim.Render(result.Message))
		} else if result.Message != "" {
			fmt.Printf("%s %s\n", statusStr, ui.Bold.Render(result.Name))
			fmt.Printf("%s%s\n", ui.Indent2, result.Message)
		} else {
			fmt.Printf("%s %s\n", statusStr, ui.Bold.Render(result.Name))
		}

		if result.Details != "" {
			for line := range strings.SplitSeq(result.Details, "\n") {
				fmt.Printf("%s%s\n", ui.Indent2, ui.Dim.Render(line))
			}
		}

		if result.Suggestion != "" {
			fmt.Printf("%s%s %s\n", ui.Indent2, ui.Dim.Render(ui.SymbolArrow), ui.Info.Render(result.Suggestion))
		}
	}

	fmt.Println()
	if doc.HasErrors() {
		ui.PrintError("Some checks failed")
		return clierrors.New(clierrors.CodeInternal, "health check failed")
	} else if doc.HasWarnings() {
		ui.PrintWarning("Some checks have warnings")
	} else {
		ui.PrintSuccess("All checks passed")
	}

	return nil
}
