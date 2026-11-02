package version

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/version"
)

type Options struct {
	JSON  bool
	Short bool
}

func Run(_ context.Context, options Options) error {
	info := version.Get()
	if options.Short {
		fmt.Println(info.Version)
		return nil
	}

	if options.JSON {
		data, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return fmt.Errorf("error encoding version info as JSON: %w", err)
		}

		ui.Println(string(data))
		return nil
	}

	ui.Printf("%s %s\n", ui.Bold.Render("xf"), ui.Version.Render(info.Version))
	ui.Println()
	var pairs []ui.KVPair
	if info.Commit != "" && info.Commit != "unknown" {
		pairs = append(pairs, ui.KV("Commit", info.Commit))
	}

	if info.Date != "" && info.Date != "unknown" {
		built := info.Date
		if t, err := time.Parse(time.RFC3339, info.Date); err == nil {
			built = ui.FormatDateTime(t)
		}

		pairs = append(pairs, ui.KV("Built", built))
	}

	pairs = append(pairs, ui.KV("Go version", info.GoVersion))
	pairs = append(pairs, ui.KV("Platform", fmt.Sprintf("%s/%s", info.OS, info.Arch)))
	ui.PrintKeyValuePadded(pairs)
	return nil
}
