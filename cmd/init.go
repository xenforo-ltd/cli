package cmd

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/initflow"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
	"github.com/xenforo-ltd/cli/internal/xfcmd"
)

// ErrUsernameTooShort is returned when username validation fails.
var (
	ErrUsernameTooShort   = errors.New("username must be at least 3 characters")
	ErrPasswordRequired   = errors.New("password is required")
	ErrInvalidEmail       = errors.New("invalid email address")
	ErrAdminUserRequired  = errors.New("admin username is required")
	ErrValidEmailRequired = errors.New("valid admin email is required")
)

var initCmd = &cobra.Command{
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
  6. Runs 'up' to start the containers
  7. Runs 'xf:install' to complete the installation

Existing Directory Mode (--existing flag):
  For core developers who already have XenForo source files checked out.
  This mode skips downloading and only sets up Docker configuration.

  1. Detects existing XenForo installation (src/XF.php)
  2. Extracts Docker configuration files
  3. Configures the .env file
  4. Optionally starts containers (with --up flag)

Examples:
  # Fresh install (interactive)
  xf init ./my-project

  # Fresh install (non-interactive)
  xf init ./my-project --license ABC123 --version 2030871 \
    --admin-user admin --admin-password secret --admin-email admin@example.com

  # Existing directory (auto-detected, no auth needed)
  xf init ./existing-xf-project --existing

  # Existing directory with custom contexts
  xf init ./existing-xf-project --existing --contexts caddy,mysql,development,redis

  # Existing directory and start containers
  xf init ./existing-xf-project --existing --up

  # Provide .env overrides
  xf init ./my-project --env-file ./my.env --env XF_TITLE="My Site"

Note: init defaults XF_DEBUG=1 and XF_DEVELOPMENT=1.
You can override either value via --env-file/--env.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

// InitOptions contains options for initialization.
type InitOptions struct {
	TargetPath       string
	LicenseKey       string
	Products         []string
	VersionID        int
	VersionString    string
	AdminUser        string
	AdminPassword    string
	AdminEmail       string
	SiteTitle        string
	InstanceName     string
	SkipUp           bool
	SkipInstall      bool
	ExistingOnly     bool
	Contexts         []string
	StartContainers  bool
	EnvFile          string
	EnvFlags         []string
	EnvResolved      map[string]string
	EnvSources       map[string]string
	ProductOverrides map[string]int
	CoreVersions     []customerapi.Version
	ProductTitleMap  map[string]string
}

var (
	flagInitLicense       string
	flagInitVersion       int
	flagInitProducts      []string
	flagInitAdminUser     string
	flagInitAdminPassword string
	flagInitAdminEmail    string
	flagInitTitle         string
	flagInitInstance      string
	flagInitSkipUp        bool
	flagInitSkipInstall   bool
	flagInitExisting      bool
	flagInitContexts      []string
	flagInitUp            bool
	flagInitEnvFile       string
	flagInitEnv           []string
)

func init() {
	initCmd.Flags().StringVar(&flagInitLicense, "license", "", "license key")
	initCmd.Flags().IntVar(&flagInitVersion, "version", 0, "version ID to install")
	initCmd.Flags().StringSliceVar(&flagInitProducts, "products", nil, "additional products to install (e.g., xfmg,xfes)")
	initCmd.Flags().StringVar(&flagInitAdminUser, "admin-user", "", "admin username")
	initCmd.Flags().StringVar(&flagInitAdminPassword, "admin-password", "", "admin password")
	initCmd.Flags().StringVar(&flagInitAdminEmail, "admin-email", "", "admin email address")
	initCmd.Flags().StringVar(&flagInitTitle, "title", "", "site title")
	initCmd.Flags().StringVar(&flagInitInstance, "instance", "", "Docker instance name")
	initCmd.Flags().BoolVar(&flagInitSkipUp, "skip-up", false, "skip starting Docker containers")
	initCmd.Flags().BoolVar(&flagInitSkipInstall, "skip-install", false, "skip running xf:install")
	initCmd.Flags().BoolVar(&flagInitExisting, "existing", false, "initialize Docker in an existing XenForo directory (skips download)")
	initCmd.Flags().StringSliceVar(&flagInitContexts, "contexts", nil, "Docker contexts to enable (e.g., caddy,mysql,development,redis)")
	initCmd.Flags().BoolVar(&flagInitUp, "up", false, "start containers after initialization (for --existing mode)")
	initCmd.Flags().StringVar(&flagInitEnvFile, "env-file", "", "path to env overrides file (KEY=VALUE lines)")
	initCmd.Flags().StringArrayVar(&flagInitEnv, "env", nil, "environment override in KEY=VALUE format (repeatable)")

	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	targetPath := "."
	if len(args) > 0 {
		targetPath = args[0]
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return clierrors.Wrap(clierrors.CodeInvalidInput, "invalid target path", err)
	}

	opts := &InitOptions{
		TargetPath:       absPath,
		LicenseKey:       flagInitLicense,
		VersionID:        flagInitVersion,
		Products:         flagInitProducts,
		AdminUser:        flagInitAdminUser,
		AdminPassword:    flagInitAdminPassword,
		AdminEmail:       flagInitAdminEmail,
		SiteTitle:        flagInitTitle,
		InstanceName:     flagInitInstance,
		SkipUp:           flagInitSkipUp,
		SkipInstall:      flagInitSkipInstall,
		ExistingOnly:     flagInitExisting,
		Contexts:         flagInitContexts,
		StartContainers:  flagInitUp,
		EnvFile:          flagInitEnvFile,
		EnvFlags:         flagInitEnv,
		ProductOverrides: map[string]int{},
		ProductTitleMap:  map[string]string{},
	}

	fileEnv := map[string]string{}
	if opts.EnvFile != "" {
		fileEnv, err = initflow.ParseEnvFile(opts.EnvFile)
		if err != nil {
			return clierrors.Wrap(clierrors.CodeInvalidInput, "failed to parse --env-file", err)
		}
	}

	flagEnv, err := initflow.ParseEnvFlags(opts.EnvFlags)
	if err != nil {
		return clierrors.Wrap(clierrors.CodeInvalidInput, "failed to parse --env", err)
	}

	opts.EnvResolved, opts.EnvSources = initflow.MergeEnvMaps(map[string]string{}, fileEnv, flagEnv)

	hasXenForo, err := detectXenForo(absPath)
	if err != nil {
		return err
	}

	if hasXenForo || opts.ExistingOnly {
		return initExisting(opts)
	}

	if err := checkPrerequisites(); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.NoInteraction {
		if err := runInteractiveSetup(ctx, opts); err != nil {
			return err
		}
	} else {
		if err := validateNonInteractiveFlags(opts); err != nil {
			return err
		}
	}

	return executeInit(ctx, opts)
}

func detectXenForo(path string) (bool, error) {
	xfPath := filepath.Join(path, "src", "XF.php")

	_, err := os.Stat(xfPath)
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to check XenForo path", err)
}

func initExisting(opts *InitOptions) error {
	fmt.Println(ui.Bold.Render("Initializing Docker environment in existing XenForo directory..."))
	fmt.Println()

	xfDir := opts.TargetPath

	if err := dockercompose.CheckDockerRunning(); err != nil {
		return err
	}

	ui.PrintSuccess("Docker is running")

	if err := dockercompose.CheckDockerComposeAvailable(); err != nil {
		return err
	}

	ui.PrintSuccess("Docker Compose is available")
	fmt.Println()

	ui.PrintStep(1, 3, "Setting up Docker configuration")

	xfcmdOpts := xfcmd.InitOptions{
		OverwriteExisting: true,
		Contexts:          opts.Contexts,
	}

	if err := xfcmd.InitExisting(xfDir, xfcmdOpts); err != nil {
		return err
	}

	ui.PrintSuccess("Docker configuration files extracted")

	ui.PrintStep(2, 3, "Configuring environment")

	if err := configureExistingEnv(opts); err != nil {
		return err
	}

	ui.PrintSuccess(fmt.Sprintf("Configured instance: %s", opts.InstanceName))

	ui.PrintStep(3, 3, "Starting environment")

	if opts.StartContainers {
		runner, err := dockercompose.NewRunner(xfDir)
		if err != nil {
			return err
		}

		if err := runner.Up(true); err != nil {
			return err
		}

		url, err := runner.GetURL()
		if err == nil && url != "" {
			ui.PrintDetail(fmt.Sprintf("Site: %s", url))
		}
	} else {
		ui.PrintDetail("Skipped (use --up flag to start containers)")
	}

	fmt.Println()
	ui.SuccessBox("Docker environment initialized!", []ui.KVPair{
		ui.KV("Location", ui.Path.Render(xfDir)),
		ui.KV("Instance", opts.InstanceName),
	})

	if !opts.StartContainers {
		fmt.Println()
		fmt.Println("To start the environment:")
		fmt.Printf("%s%s\n", ui.Indent1, ui.Command.Render(fmt.Sprintf("cd %s", xfDir)))
		fmt.Printf("%s%s\n", ui.Indent1, ui.Command.Render("xf up"))
	}

	fmt.Println()
	printUsefulCommands()

	return nil
}

func configureExistingEnv(opts *InitOptions) error {
	envPath := filepath.Join(opts.TargetPath, ".env")

	if opts.InstanceName == "" {
		dirName := filepath.Base(opts.TargetPath)
		opts.InstanceName = xf.GenerateInstanceName(dirName)
	}

	contexts := opts.Contexts
	if len(contexts) == 0 {
		contexts = []string{"caddy", "mysql", "development", "caddy-development", "redis", "mailpit"}
	}

	updates := map[string]string{
		"XF_INSTANCE":    opts.InstanceName,
		"XF_CONTEXTS":    strings.Join(contexts, ":"),
		"XF_TITLE":       fmt.Sprintf("XenForo [%s]", opts.InstanceName),
		"XF_EMAIL":       "admin@example.com",
		"XF_DEBUG":       "1",
		"XF_DEVELOPMENT": "1",
	}
	maps.Copy(updates, opts.EnvResolved)

	if err := xf.WriteEnvFile(envPath, updates); err != nil {
		return err
	}

	return nil
}

func checkPrerequisites() error {
	fmt.Println(ui.Bold.Render("Checking prerequisites..."))

	if err := dockercompose.CheckDockerRunning(); err != nil {
		return err
	}

	ui.PrintSuccess("Docker is running")

	if err := dockercompose.CheckDockerComposeAvailable(); err != nil {
		return err
	}

	ui.PrintSuccess("Docker Compose is available")

	fmt.Println()

	return nil
}

func validateNonInteractiveFlags(opts *InitOptions) error {
	var missing []string

	if opts.LicenseKey == "" {
		missing = append(missing, "--license")
	}

	if opts.VersionID == 0 {
		missing = append(missing, "--version")
	}

	if opts.AdminUser == "" {
		missing = append(missing, "--admin-user")
	}

	if opts.AdminPassword == "" {
		missing = append(missing, "--admin-password")
	}

	if opts.AdminEmail == "" {
		missing = append(missing, "--admin-email")
	}

	if len(missing) > 0 {
		return clierrors.Newf(clierrors.CodeInvalidInput, "missing required flags in non-interactive mode: %s", strings.Join(missing, ", "))
	}

	if len(opts.Products) == 0 {
		opts.Products = []string{"xenforo"}
	} else {
		opts.Products = ensureCoreFirstUnique(opts.Products)
	}

	return nil
}

func runInteractiveSetup(ctx context.Context, opts *InitOptions) error {
	client, err := customerapi.NewClient()
	if err != nil {
		return err
	}

	if title := inferSiteTitleFromEnv(opts); title != "" {
		opts.SiteTitle = title
	}

	if opts.LicenseKey == "" {
		licenses, err := client.GetLicenses(ctx)
		if err != nil {
			return err
		}

		if len(licenses) == 0 {
			return clierrors.New(clierrors.CodeAPINotFound, "no licenses found for your account")
		}

		var licenseOptions []huh.Option[string]

		for _, lic := range licenses {
			if lic.CanDownload {
				label := licenseOptionLabel(lic)
				licenseOptions = append(licenseOptions, huh.NewOption(label, lic.LicenseKey))
			}
		}

		if len(licenseOptions) == 0 {
			return clierrors.New(clierrors.CodeAPIForbidden, "no licenses with download access found")
		}

		err = huh.NewSelect[string]().
			Title("Select a license").
			Options(licenseOptions...).
			Value(&opts.LicenseKey).
			Run()
		if err != nil {
			return clierrors.Wrap(clierrors.CodeInvalidInput, "license selection cancelled", err)
		}
	}

	if len(opts.Products) == 0 {
		downloadables, err := client.GetLicenseDownloadables(ctx, opts.LicenseKey)
		if err != nil {
			return err
		}

		var productOptions []huh.Option[string]

		for _, d := range downloadables.Downloadables {
			if d.DownloadID == "xenforo" {
				continue
			}

			productOptions = append(productOptions, huh.NewOption(d.Title, d.DownloadID))
		}

		var selectedProducts []string

		err = huh.NewMultiSelect[string]().
			Title("What additional products should be installed?").
			Description("XenForo core is always installed. Use ↑/↓ to move, Space to select, Enter to continue.").
			Options(productOptions...).
			Value(&selectedProducts).
			Run()
		if err != nil {
			return clierrors.Wrap(clierrors.CodeInvalidInput, "product selection cancelled", err)
		}

		opts.Products = ensureCoreFirstUnique(append([]string{"xenforo"}, selectedProducts...))
	}

	if len(opts.Products) > 0 {
		opts.Products = ensureCoreFirstUnique(opts.Products)
	}

	versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, "xenforo")
	if err != nil {
		return err
	}

	if len(versions.Versions) == 0 {
		return clierrors.New(clierrors.CodeAPINotFound, "no versions available")
	}

	initflow.SortVersionsDesc(versions.Versions)
	opts.CoreVersions = versions.Versions

	if opts.VersionID == 0 {
		if err := chooseCoreVersionInteractively(opts); err != nil {
			return err
		}
	}

	if opts.VersionString == "" {
		for _, v := range opts.CoreVersions {
			if v.VersionID == opts.VersionID {
				opts.VersionString = v.VersionStr
				break
			}
		}
	}

	if opts.VersionID == 0 {
		return clierrors.New(clierrors.CodeInvalidInput, "core version is required")
	}

	if opts.AdminUser == "" || opts.AdminPassword == "" || opts.AdminEmail == "" {
		if opts.AdminUser == "" {
			opts.AdminUser = "admin"
		}

		if opts.AdminEmail == "" {
			opts.AdminEmail = "admin@example.com"
		}

		if opts.SiteTitle == "" {
			opts.SiteTitle = "XenForo"
		}

		form := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Admin username").
					Value(&opts.AdminUser).
					Validate(func(s string) error {
						if len(s) < 3 {
							return ErrUsernameTooShort
						}

						return nil
					}),
				huh.NewInput().
					Title("Admin password").
					Value(&opts.AdminPassword).
					EchoMode(huh.EchoModePassword).
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return ErrPasswordRequired
						}

						return nil
					}),
				huh.NewInput().
					Title("Admin email").
					Value(&opts.AdminEmail).
					Validate(func(s string) error {
						if !strings.Contains(s, "@") {
							return ErrInvalidEmail
						}

						return nil
					}),
			),
		)

		if err := form.Run(); err != nil {
			return clierrors.Wrap(clierrors.CodeInvalidInput, "credential input cancelled", err)
		}
	}

	if opts.InstanceName == "" {
		opts.InstanceName = xf.GenerateInstanceName(filepath.Base(opts.TargetPath))
	}

	if err := runInteractiveReview(ctx, client, opts); err != nil {
		return err
	}

	return nil
}
