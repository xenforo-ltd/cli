package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/initflow"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
	"github.com/xenforo-ltd/cli/internal/xfcmd"
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
  7. Runs 'composer install' if composer.json is present
  8. Runs 'xf:install' to complete the installation

Existing Directory Mode (--existing flag):
  For core developers who already have XenForo source files checked out.
  This mode skips downloading and only sets up Docker configuration.

  1. Detects existing XenForo installation (src/XF.php)
  2. Extracts Docker configuration files
  3. Configures the .env file
  4. Optionally starts containers (with --up flag)

  Repository checkouts track composer.json, so dependencies are installed
  automatically once the containers are running. Release packages ship
  vendor/ prebuilt and have no manifest, so they are skipped.

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
  xf init ./existing-xf-project --existing --contexts caddy,mysql,development,redis

  # Existing directory and start containers
  xf init ./existing-xf-project --existing --up

  # Provide .env overrides
  xf init ./my-project --env-file ./my.env --env XF_TITLE="My Site"`,
	Args:    cobra.MaximumNArgs(1),
	GroupID: "start",
	RunE:    runInit,
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
	SkipComposer     bool
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
	flagInitSkipComposer  bool
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
	initCmd.Flags().BoolVar(&flagInitSkipComposer, "skip-composer", false, "skip running composer install")
	initCmd.Flags().BoolVar(&flagInitExisting, "existing", false, "initialize Docker in an existing XenForo directory (skips download)")
	initCmd.Flags().StringSliceVar(&flagInitContexts, "contexts", nil, "Docker contexts to enable (e.g., caddy,mysql,development,redis)")
	initCmd.Flags().BoolVar(&flagInitUp, "up", false, "start containers after initialization (for --existing mode)")
	initCmd.Flags().StringVar(&flagInitEnvFile, "env-file", "", "path to env overrides file (KEY=VALUE lines)")
	initCmd.Flags().StringArrayVar(&flagInitEnv, "env", nil, "environment override in KEY=VALUE format (repeatable)")

	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Minute)
	defer cancel()

	targetPath := "."
	if len(args) > 0 {
		targetPath = args[0]
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
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
		SkipComposer:     flagInitSkipComposer,
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
			return fmt.Errorf("failed to parse --env-file: %w", err)
		}
	}

	flagEnv, err := initflow.ParseEnvFlags(opts.EnvFlags)
	if err != nil {
		return fmt.Errorf("failed to parse --env: %w", err)
	}

	opts.EnvResolved, opts.EnvSources = initflow.MergeEnvMaps(map[string]string{}, fileEnv, flagEnv)

	hasXenForo, err := detectXenForo(absPath)
	if err != nil {
		return err
	}

	if hasXenForo || opts.ExistingOnly {
		return initExisting(ctx, opts)
	}

	if err := checkPrerequisites(ctx); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
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

	return false, fmt.Errorf("failed to check XenForo path: %w", err)
}

// validateAdminDetails reports whether the installer has everything it needs.
//
// installExistingXenForo passes these straight to xf:install, so a missing
// value would install a broken administrator or an empty board title instead
// of failing.
func validateAdminDetails(opts *InitOptions) error {
	var missing []string

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
		return newUsageError(fmt.Errorf(
			"missing required flags for the XenForo installation: %s: %w",
			strings.Join(missing, ", "), ErrInvalidInput,
		))
	}

	return nil
}

func initExisting(ctx context.Context, opts *InitOptions) error {
	ui.Println(ui.Header.Render("Initializing Docker environment"))
	ui.Println()

	xfDir := opts.TargetPath

	if err := checkPrerequisites(ctx); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	step := 1
	totalSteps := 3

	ui.PrintStep(step, totalSteps, "Setting up Docker configuration")
	step++

	xfcmdOpts := xfcmd.InitOptions{
		OverwriteExisting: true,
		Contexts:          opts.Contexts,
	}

	writtenDefaults, err := xfcmd.InitExisting(xfDir, xfcmdOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker files in existing XenForo directory: %w", err)
	}

	ui.PrintSuccess("Docker configuration files extracted")

	for _, p := range writtenDefaults {
		ui.PrintInfo("Updated defaults written to " + ui.Path.Render(p))
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Configuring environment")
	step++

	if err := configureExistingEnv(opts); err != nil {
		return err
	}

	ui.PrintSubstep("Configured instance: " + ui.Bold.Render(opts.InstanceName))

	ui.Println()

	// Detection can fail or return nothing, and installing --url= empty would
	// leave the board with no address at all, so the predictable instance URL
	// is the starting point.
	siteURL := fallbackBoardURL(opts.InstanceName)

	if opts.StartContainers {
		ui.PrintStep(step, totalSteps, "Starting environment")

		runner, err := dockercompose.NewRunner(xfDir)
		if err != nil {
			return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
		}

		if err := runner.Up(ctx, true); err != nil {
			return fmt.Errorf("failed to start Docker environment: %w", err)
		}

		url, err := runner.GetURL(ctx)
		if err == nil && url != "" {
			siteURL = url
		}

		// Composer and the installer both run inside the container, so they can
		// only follow a successful start.
		if shouldRunComposer(xfDir) && !opts.SkipComposer {
			ui.Println()

			if err := runComposerInstall(ctx, runner, cfg.Verbose); err != nil {
				return err
			}
		}

		if !opts.SkipInstall && opts.AdminUser != "" {
			// The installer receives these verbatim, so an empty value becomes
			// an empty board title or an unusable administrator rather than a
			// reported error.
			if opts.SiteTitle == "" {
				opts.SiteTitle = opts.EnvResolved["XF_TITLE"]
			}

			if opts.SiteTitle == "" {
				opts.SiteTitle = fmt.Sprintf("XenForo [%s]", opts.InstanceName)
			}

			if err := validateAdminDetails(opts); err != nil {
				return err
			}

			ui.Println()

			if err := installExistingXenForo(ctx, runner, opts, siteURL, cfg.Verbose); err != nil {
				return err
			}
		}
	} else {
		printSkippedStep(step, totalSteps, "Starting environment", "use --up to start containers")
	}

	details := []ui.KVPair{
		ui.KV("Location", ui.Path.Render(ui.ShortHome(xfDir))),
		ui.KV("Instance", opts.InstanceName),
	}
	if siteURL != "" {
		details = append(details, ui.KV("URL", ui.URL.Render(siteURL)))
	}

	ui.Println()
	ui.SuccessBox("Docker environment initialized", details)

	if !opts.StartContainers {
		ui.Println()
		printStartHint(xfDir)
	}

	ui.Println()
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
		return fmt.Errorf("failed to write environment configuration: %w", err)
	}

	return nil
}

func checkPrerequisites(ctx context.Context) error {
	ui.Println(ui.Bold.Render("Checking prerequisites..."))

	if err := dockercompose.CheckDockerRunning(ctx); err != nil {
		return withHint(
			markAs(ErrInternal, "Docker does not appear to be running"),
			"Start Docker Desktop and try again",
		)
	}

	ui.PrintSuccess("Docker is running")

	if err := dockercompose.CheckDockerComposeAvailable(ctx); err != nil {
		return fmt.Errorf("failed to verify Docker Compose is available: %w", err)
	}

	ui.PrintSuccess("Docker Compose is available")

	ui.Println()

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
		return newUsageError(fmt.Errorf("missing required flags in non-interactive mode: %s: %w", strings.Join(missing, ", "), ErrInvalidInput))
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
		return fmt.Errorf("failed to create customer API client: %w", err)
	}

	if title := inferSiteTitleFromEnv(opts); title != "" {
		opts.SiteTitle = title
	}

	if opts.LicenseKey == "" {
		licenses, err := client.GetLicenses(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch licenses: %w", err)
		}

		if len(licenses) == 0 {
			return markAs(ErrNotFound, "no licenses found for your account")
		}

		var licenseOptions []huh.Option[string]

		for _, lic := range licenses {
			if lic.CanDownload {
				label := licenseLabel(lic)
				licenseOptions = append(licenseOptions, huh.NewOption(label, lic.LicenseKey))
			}
		}

		if len(licenseOptions) == 0 {
			return markAs(ErrForbidden, "no licenses with download access found")
		}

		err = huh.NewSelect[string]().
			Title("Select a license").
			Options(licenseOptions...).
			Value(&opts.LicenseKey).
			Run()
		if err != nil {
			return markAs(ErrCancelled, "license selection cancelled")
		}
	}

	if len(opts.Products) == 0 {
		downloadables, err := client.GetLicenseDownloadables(ctx, opts.LicenseKey)
		if err != nil {
			return fmt.Errorf("failed to fetch available downloads for license %s: %w", opts.LicenseKey, err)
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
			return markAs(ErrCancelled, "product selection cancelled")
		}

		opts.Products = ensureCoreFirstUnique(append([]string{"xenforo"}, selectedProducts...))
	}

	if len(opts.Products) > 0 {
		opts.Products = ensureCoreFirstUnique(opts.Products)
	}

	versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, "xenforo")
	if err != nil {
		return fmt.Errorf("failed to fetch XenForo versions for license %s: %w", opts.LicenseKey, err)
	}

	if len(versions.Versions) == 0 {
		return markAs(ErrNotFound, "no versions available")
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
		return markAs(ErrInvalidInput, "core version is required")
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
						if len(s) < minimumUsernameLength {
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
			return markAs(ErrCancelled, "credential input cancelled")
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

// installExistingXenForo runs xf:install in an already-configured environment.
//
// The password is passed through the environment rather than the command line,
// so it does not appear in the container's process list.
func installExistingXenForo(
	ctx context.Context,
	runner *dockercompose.Runner,
	opts *InitOptions,
	siteURL string,
	verbose bool,
) error {
	if err := runner.WaitForDatabase(ctx, 2*time.Second); err != nil {
		return fmt.Errorf("failed waiting for database to become ready: %w", err)
	}

	installArgs := []string{
		"xf:install",
		"--no-interaction",
		"--clear",
		"--user=" + opts.AdminUser,
		"--email=" + opts.AdminEmail,
		"--title=" + opts.SiteTitle,
		"--url=" + siteURL,
	}

	installEnv := map[string]string{"XF_INSTALL_PASSWORD": opts.AdminPassword}
	shellInstallArgs := []string{"sh", "-c", installShellCommand(installArgs)}

	if verbose {
		ui.PrintSubstep("Running XenForo installation...")

		if err := runner.ExecOrRunWithEnv(ctx, "xf", true, installEnv, shellInstallArgs...); err != nil {
			return fmt.Errorf("failed to install XenForo: %w", err)
		}

		return nil
	}

	spinner := ui.NewSpinner("Installing XenForo...")
	spinner.Start()

	tracker := newPhaseTrackerWriter(spinner, "Installing XenForo", installPhaseRules())

	if err := runner.ExecOrRunWithEnvAndOutput(ctx, "xf", true, installEnv, tracker, tracker, shellInstallArgs...); err != nil {
		spinner.Stop()
		printHiddenOutputTail("Installer output", tracker.TailLines())

		return fmt.Errorf("failed to install XenForo: %w", err)
	}

	spinner.StopWithMessage("success", "XenForo installed")

	return nil
}
