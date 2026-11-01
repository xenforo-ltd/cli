package main

import (
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/commands/doctor"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check system health and dependencies",
	Long: `Run diagnostic checks to verify that your system is properly configured
for XenForo CLI.

This command checks:
  - Credential storage availability
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

func init() { rootCmd.AddCommand(doctorCmd) }

func runDoctor(cmd *cobra.Command, args []string) error {
	return doctor.Run(cmd.Context())
}
