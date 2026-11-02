package doctor

import (
	"context"
	"fmt"
	"strings"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func Run(ctx context.Context) error {
	doc := newDoctor()

	ui.Println(ui.Header.Render("System health check"))
	ui.Println()

	results := doc.runAll(ctx)

	var passed, warned, failed, skipped int

	for _, result := range results {
		var statusStr string

		switch result.status {
		case statusOK:
			statusStr = ui.StatusIcon("success")
			passed++
		case statusWarning:
			statusStr = ui.StatusIcon("warning")
			warned++
		case statusError:
			statusStr = ui.StatusIcon("error")
			failed++
		case statusSkipped:
			statusStr = ui.StatusIcon("skipped")
			skipped++
		default:
			statusStr = ui.StatusIcon("pending")
		}

		line := fmt.Sprintf("%s %s", statusStr, ui.Bold.Render(result.name))
		if result.message != "" {
			line += "  " + ui.Dim.Render(result.message)
		}

		ui.Println(line)

		if result.details != "" {
			for detail := range strings.SplitSeq(result.details, "\n") {
				ui.Printf("%s%s\n", ui.Indent1, ui.Dim.Render(detail))
			}
		}

		if result.suggestion != "" {
			ui.PrintHint(result.suggestion)
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
		ui.PrintError(fmt.Sprintf("%s failed (%d passed, %s%s)",
			ui.Plural(failed, "check", "checks"), passed,
			ui.Plural(warned, "warning", "warnings"), skippedSuffix))

		return clierror.WithExitCode(1)
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
