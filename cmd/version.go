package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/version"
)

var (
	flagVersionJSON  bool
	flagVersionShort bool
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long: `Display the version, build commit, build date, and runtime information.

Shows detailed build information including the exact Git commit and
build timestamp when available.

Examples:
  # Show full version info
  xf version

  # Show only version number
  xf version --short

  # Output as JSON (useful for scripting)
  xf version --json`,
	Run: func(cmd *cobra.Command, args []string) {
		info := version.Get()

		if flagVersionShort {
			ui.Println(ui.Version.Render(info.Short()))
			return
		}

		if flagVersionJSON {
			data, err := json.MarshalIndent(info, "", "  ")
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				return
			}

			fmt.Println(string(data))

			return
		}

		ui.Printf("%s %s\n\n", ui.Bold.Render("xf"), ui.Version.Render(info.Version))

		var pairs []ui.KVPair
		if info.Commit != "" && info.Commit != "unknown" {
			pairs = append(pairs, ui.KV("Commit", ui.Dim.Render(info.Commit)))
		}

		if info.Date != "" && info.Date != "unknown" {
			pairs = append(pairs, ui.KV("Built", info.Date))
		}

		pairs = append(pairs, ui.KV("Go version", info.GoVersion))
		pairs = append(pairs, ui.KV("Platform", fmt.Sprintf("%s/%s", info.OS, info.Arch)))

		ui.PrintKeyValuePadded(pairs)
	},
}

func init() {
	versionCmd.Flags().BoolVar(&flagVersionJSON, "json", false, "output as JSON")
	versionCmd.Flags().BoolVarP(&flagVersionShort, "short", "s", false, "print only the version number")
	rootCmd.AddCommand(versionCmd)
}
