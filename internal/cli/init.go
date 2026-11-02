package cli

import (
	"github.com/spf13/cobra"
	"github.com/xenforo-ltd/cli/internal/commands/initialize"
	"github.com/xenforo-ltd/cli/internal/completion"
)

// initOptions holds the flags for environment initialization.
var initOptions initialize.InitOptions

// legacyInitContexts holds values of the deprecated --contexts flag, which is
// kept for one release so existing scripts keep working.
var legacyInitContexts []string

// newInitCommand builds the init command.
func newInitCommand() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a new XenForo development environment",
		Long: `Initialize a new XenForo development environment.

This command can operate in two modes:

Fresh Install Mode (default):
  1. Prompts for license, products, version, and admin credentials
  2. Downloads XenForo files to the local cache
  3. Extracts XenForo files to the target directory
  4. Sets up Docker configuration
  5. Configures the .env file
  6. Builds JavaScript for XF and add-ons with build.json
  7. Runs 'up' to start the containers
  8. Runs 'composer install' if composer.json is present
  9. Runs 'xf:install' to complete the installation

Existing Directory Mode (--existing flag):
  For core developers who already have XenForo source files checked out.
  This mode skips downloading and only sets up Docker configuration.

  1. Detects existing XenForo installation (src/XF.php)
  2. Extracts Docker configuration files
  3. Configures the .env file
  4. Builds JavaScript for XF and add-ons with build.json
  5. Optionally starts containers (with --up flag)

  Repository checkouts track composer.json, so dependencies are installed
  automatically once the containers are running, along with those of any
  add-on that keeps its own in a gitignored _vendor directory. Release
  packages ship vendor/ prebuilt and have no manifest, so they are skipped.

  JavaScript is rebuilt for XF and each add-on with a build.json, even
  without starting containers. Release packages omit this file and are unchanged.
  Use --skip-build-js to skip these builds independently of Composer and
  the XenForo installer.

Note: init defaults XF_DEBUG=1 and XF_DEVELOPMENT=1.
You can override either value via --env-file/--env.`,
		Example: `  # Fresh install (interactive)
  xf init ./my-project

  # Fresh install (non-interactive)
  xf init ./my-project --license ABC123 --version 2030871 \
    --admin-user admin --admin-password secret --admin-email admin@example.com

  # Existing directory (auto-detected, no auth needed)
  xf init ./existing-xf-project --existing

  # Existing directory with custom contexts
  xf init ./existing-xf-project --existing --context caddy --context mysql --context development

  # Existing directory and start containers
  xf init ./existing-xf-project --existing --up

  # Provide .env overrides
  xf init ./my-project --env-file ./my.env --env XF_TITLE="My Site"`,
		Args: cobra.MaximumNArgs(1), GroupID: "start", RunE: runInit,
	}

	initCmd.Flags().StringVar(&initOptions.LicenseKey, "license", "", "license key")
	initCmd.Flags().IntVar(&initOptions.VersionID, "version", 0, "version ID to install")
	initCmd.Flags().StringSliceVar(&initOptions.Products, "products", nil, "additional products to install (e.g., xfmg,xfes)")
	initCmd.Flags().StringVar(&initOptions.AdminUser, "admin-user", "", "admin username")
	initCmd.Flags().StringVar(&initOptions.AdminPassword, "admin-password", "", "admin password")
	initCmd.Flags().StringVar(&initOptions.AdminEmail, "admin-email", "", "admin email address")
	initCmd.Flags().StringVar(&initOptions.SiteTitle, "title", "", "site title")
	initCmd.Flags().StringVar(&initOptions.InstanceName, "instance", "", "Docker instance name")
	initCmd.Flags().BoolVar(&initOptions.SkipUp, "skip-up", false, "skip starting Docker containers")
	initCmd.Flags().BoolVar(&initOptions.SkipInstall, "skip-install", false, "skip running xf:install")
	initCmd.Flags().BoolVar(&initOptions.SkipComposer, "skip-composer", false, "skip running composer install")
	initCmd.Flags().BoolVar(&initOptions.SkipBuildJS, "skip-build-js", false, "skip building JavaScript for XF and add-ons")
	initCmd.Flags().BoolVar(&initOptions.ExistingOnly, "existing", false, "initialize Docker in an existing XenForo directory (skips download)")
	initCmd.Flags().StringArrayVar(&initOptions.Contexts, "context", nil, "Docker context to enable (repeatable)")
	initCmd.Flags().StringSliceVar(&legacyInitContexts, "contexts", nil, "Docker contexts to enable (deprecated)")
	cobra.CheckErr(initCmd.Flags().MarkDeprecated("contexts", "use --context, which can be repeated"))
	initCmd.Flags().BoolVar(&initOptions.StartContainers, "up", false, "start containers after initialization (for --existing mode)")
	initCmd.Flags().StringVar(&initOptions.EnvFile, "env-file", "", "path to env overrides file (KEY=VALUE lines)")
	initCmd.Flags().StringArrayVar(&initOptions.EnvFlags, "env", nil, "environment override in KEY=VALUE format (repeatable)")
	completion.Register(initCmd, "license", completion.LicenseKeys)
	completion.Register(initCmd, "context", completion.DockerContexts)

	return initCmd
}

// runInit initializes the target directory, defaulting to the current directory.
// Deprecated comma-separated contexts are appended after repeated --context values.
func runInit(cmd *cobra.Command, args []string) error {
	opts := initOptions
	opts.TargetPath = "."
	if len(args) > 0 {
		opts.TargetPath = args[0]
	}

	opts.Contexts = append(opts.Contexts, legacyInitContexts...)

	return initialize.Run(cmd.Context(), &opts)
}
