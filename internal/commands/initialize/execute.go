package initialize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/setup"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
)

// downloadSpinnerStep is the byte interval between spinner updates when the
// download size is unknown.
const downloadSpinnerStep = 100 * 1024

// plannedInitSteps returns the number of steps that will be printed for a
// fresh install run with the given options. Every step that runs, or that is
// reachable but skipped (and therefore printed via printSkippedStep), counts
// toward the total.
//
// JavaScript always occupies a slot. Container startup, Composer and installation
// only occupy slots when startup is requested, whether they run or are skipped.
// The total does not depend on what the downloaded package contains.
func plannedInitSteps(opts InitOptions) int {
	// Preparing target directory, Downloading XenForo files, Extracting
	// XenForo files, Setting up Docker configuration, Configuring
	// environment, Building JavaScript.
	const base = 6

	if opts.SkipUp {
		return base
	}

	return base + 3
}

func executeInit(ctx context.Context, opts *InitOptions) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if opts.InstanceName == "" {
		opts.InstanceName = environment.GenerateInstanceName(filepath.Base(opts.TargetPath))
	}

	if opts.SiteTitle == "" {
		opts.SiteTitle = "XenForo"
	}

	client, err := customerapi.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create customer API client: %w", err)
	}

	titleMap := downloads.ProductTitles(ctx, client, opts.LicenseKey)

	totalSteps := plannedInitSteps(*opts)
	step := 1

	ui.PrintStep(step, totalSteps, "Preparing target directory")
	ui.Printf("%s%s\n", ui.Indent2, ui.Path.Render(ui.ShortHome(opts.TargetPath)))

	step++

	if err := prepareTargetDirectory(opts.TargetPath); err != nil {
		return err
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Downloading XenForo files")
	step++

	cachedFiles, err := downloadProducts(ctx, client, opts)
	if err != nil {
		return err
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Extracting XenForo files")
	step++

	if err := downloads.ExtractCachedFiles(cachedFiles, opts.TargetPath, titleMap, "Extracted"); err != nil {
		return err
	}

	if err := normalizeRuntimeStorage(opts.TargetPath); err != nil {
		return err
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Setting up Docker configuration")
	step++

	environmentOpts := environment.InitOptions{
		OverwriteExisting: true,
		Contexts:          opts.Contexts,
	}
	writtenDefaults, err := environment.Init(opts.TargetPath, environmentOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker configuration: %w", err)
	}

	ui.PrintSuccess("Docker configuration ready")

	for _, p := range writtenDefaults {
		ui.PrintInfo("Updated defaults written to " + ui.Path.Render(p))
	}

	meta := &xf.Metadata{
		LicenseKey:         opts.LicenseKey,
		InstanceName:       opts.InstanceName,
		InstalledProducts:  opts.Products,
		InstalledVersion:   opts.versionString,
		InstalledVersionID: opts.VersionID,
	}
	if err := xf.WriteMetadata(opts.TargetPath, meta); err != nil {
		// Non-fatal - warn but continue
		ui.PrintWarning("Could not write metadata")
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Configuring environment")
	step++

	if err := configureEnvironment(opts); err != nil {
		return err
	}

	ui.PrintSuccess("Environment configured")

	ui.Println()

	if err := setup.RunJavaScript(ctx, &setup.Options{TargetPath: opts.TargetPath, SkipBuildJS: opts.SkipBuildJS}, step, totalSteps); err != nil {
		return err
	}

	step++
	ui.Println()

	siteURL := setup.FallbackBoardURL(opts.InstanceName)

	if opts.SkipUp {
		ui.PrintInfo("Docker containers not started (--skip-up)")
	} else {
		ui.PrintStep(step, totalSteps, "Starting Docker environment")
		step++

		runner, err := environment.NewRunner(opts.TargetPath)
		if err != nil {
			return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
		}

		if err := startContainers(ctx, runner, cfg.Verbose); err != nil {
			return err
		}

		detectedURL, detectedErr := runner.GetURL(ctx)

		var detected bool

		siteURL, detected = setup.ChooseBoardURL(opts.InstanceName, detectedURL, detectedErr)
		if !detected && detectedErr != nil {
			ui.PrintWarning("Could not detect the site URL; using " + ui.URL.Render(siteURL))
		}

		// The Composer step always occupies its slot once containers start,
		// so the plan stays fixed regardless of what the extracted package
		// contains. The filesystem is authoritative here: release packages
		// ship vendor/ prebuilt and no manifest, repository checkouts track
		// composer.json.
		ui.Println()

		switch {
		case opts.SkipComposer:
			setup.PrintSkippedStep(step, totalSteps, "Installing Composer dependencies", "--skip-composer")
		case setup.ShouldRunComposer(opts.TargetPath):
			ui.PrintStep(step, totalSteps, "Installing Composer dependencies")

			if err := setup.RunComposerInstall(ctx, runner, opts.TargetPath, cfg.Verbose); err != nil {
				return err
			}
		default:
			setup.PrintSkippedStep(step, totalSteps, "Installing Composer dependencies", "composer.json not present")
		}

		step++

		ui.Println()

		if opts.SkipInstall {
			setup.PrintSkippedStep(step, totalSteps, "Installing XenForo", "--skip-install")
		} else {
			ui.PrintStep(step, totalSteps, "Installing XenForo")

			if err := installXenForo(ctx, runner, opts, siteURL, cfg.Verbose); err != nil {
				return err
			}
		}
	}

	successDetails := []ui.KVPair{
		ui.KV("Location", ui.Path.Render(ui.ShortHome(opts.TargetPath))),
		ui.KV("Instance", opts.InstanceName),
		ui.KV("Products", formatProductNames(opts.Products, titleMap)),
	}
	if !opts.SkipUp {
		successDetails = append(successDetails, ui.KV("URL", ui.URL.Render(siteURL)))
	}

	ui.Println()
	ui.SuccessBox("XenForo development environment initialized", successDetails)

	if opts.SkipUp {
		ui.Println()
		setup.PrintStartHint(opts.TargetPath)
	}

	ui.Println()
	setup.PrintUsefulCommands()

	return nil
}

func startContainers(ctx context.Context, runner *environment.Runner, verbose bool) error {
	if verbose {
		ui.PrintSubstep("Running docker compose up...")

		if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, "up", "--detach"); err != nil {
			return fmt.Errorf("failed to start Docker environment: %w", err)
		}

		return nil
	}

	spinner := ui.NewSpinner("Starting Docker environment")
	spinner.Start()

	tracker := ui.NewPhaseTrackerWriter(spinner, "Starting Docker environment", ui.DockerStartPhaseRules())
	if err := runner.Compose(ctx, os.Stdin, tracker, tracker, "up", "--detach"); err != nil {
		spinner.StopWithMessage("error", "Failed to start containers")
		ui.PrintHiddenOutputTail("Docker output", tracker.TailLines())

		return fmt.Errorf("failed to start Docker environment: %w", err)
	}

	spinner.StopWithMessage("success", "Docker containers started")

	return nil
}

func installXenForo(ctx context.Context, runner *environment.Runner, opts *InitOptions, siteURL string, verbose bool) error {
	shellInstallArgs, installEnv := setup.InstallInvocation(opts.AdminUser, opts.AdminEmail, opts.SiteTitle, siteURL, opts.AdminPassword)

	if verbose {
		ui.PrintSubstep("Waiting for the database to be ready...")

		if err := runner.WaitForDatabase(ctx, 2*time.Second); err != nil {
			return fmt.Errorf("failed waiting for database to become ready: %w", err)
		}

		ui.PrintSubstep("Running XenForo installation...")

		if err := runner.ExecOrRun(ctx, "xf", installEnv, os.Stdin, os.Stdout, os.Stderr, shellInstallArgs...); err != nil {
			return printInstallFailure(ctx, err)
		}

		return nil
	}

	spinner := ui.NewSpinner("Waiting for the database")
	spinner.Start()

	if err := runner.WaitForDatabase(ctx, 2*time.Second); err != nil {
		spinner.Stop()

		return fmt.Errorf("failed waiting for database to become ready: %w", err)
	}

	spinner.UpdateMessage("Installing XenForo")

	tracker := ui.NewPhaseTrackerWriter(spinner, "Installing XenForo", ui.InstallPhaseRules())
	if err := runner.ExecOrRun(ctx, "xf", installEnv, os.Stdin, tracker, tracker, shellInstallArgs...); err != nil {
		spinner.Stop()

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		ui.PrintHiddenOutputTail("Installer output", tracker.TailLines())

		return printInstallFailure(ctx, err)
	}

	spinner.StopWithMessage("success", "XenForo installed")

	return nil
}

// prepareTargetDirectory validates the target-directory precondition and
// acts on it. The target must not exist, or must be an empty directory, or
// must be a non-empty directory that already looks like a XenForo install;
// anything else is rejected as invalid input. It prints the outcome so the
// "Preparing target directory" step reports what happened, and it runs before
// any package is downloaded, so a doomed run fails before paying for one.
func prepareTargetDirectory(targetPath string) error {
	info, err := os.Stat(targetPath)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(targetPath, 0o755); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}

		ui.PrintSubstep("Created directory: " + ui.Path.Render(targetPath))

		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to check target directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("target path exists but is not a directory: %w", clierror.ErrInvalidInput)
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return fmt.Errorf("failed to read target directory: %w", err)
	}

	nonHiddenCount := 0

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			nonHiddenCount++
		}
	}

	if nonHiddenCount == 0 {
		ui.PrintSubstep("Directory is empty and ready")

		return nil
	}

	hasXenForo, err := detectXenForo(targetPath)
	if err != nil {
		return err
	}

	if !hasXenForo {
		return fmt.Errorf(
			"target directory is not empty (%d visible items); use an empty directory or an existing XenForo directory: %w",
			nonHiddenCount,
			clierror.ErrInvalidInput,
		)
	}

	ui.PrintWarning("Directory already contains a XenForo installation")
	ui.PrintDetail("Only Docker configuration files will be updated")

	return nil
}

func downloadProducts(ctx context.Context, client *customerapi.Client, opts *InitOptions) (map[string]*cache.Entry, error) {
	cacheManager, err := cache.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache manager: %w", err)
	}

	titleMap := downloads.ProductTitles(ctx, client, opts.LicenseKey)

	cachedFiles := make(map[string]*cache.Entry)

	productTitle := func(product string) string {
		if title := titleMap[product]; title != "" {
			return title
		}

		return product
	}

	selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.versionString, opts.productOverrides, func(product string) {
		ui.PrintWarning("No versions available for " + productTitle(product) + ", skipping")
	})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve product selections for license %s: %w", opts.LicenseKey, err)
	}

	for _, selection := range selections {
		productName := productTitle(selection.Product)

		var (
			progressBar *ui.ProgressBar
			lastUpdate  int64
		)

		spinner := ui.NewSpinner(fmt.Sprintf("Downloading %s %s", productName, selection.VersionString))
		spinner.Start()

		progress := func(current, total int64) {
			if total > 0 {
				if spinner != nil {
					spinner.Stop()
					spinner = nil
				}

				if progressBar == nil {
					label := fmt.Sprintf("%s %s", productName, selection.VersionString)
					progressBar = ui.NewProgressBar(total, label)
				}

				progressBar.Update(current)
			} else if current-lastUpdate >= downloadSpinnerStep || lastUpdate == 0 {
				lastUpdate = current

				msg := fmt.Sprintf("Downloading %s %s (%s)", productName, selection.VersionString, ui.FormatBytes(current))
				if spinner == nil {
					spinner = ui.NewSpinner(msg)
					spinner.Start()
				} else {
					spinner.UpdateMessage(msg)
				}
			}
		}

		entry, versionStr, err := downloads.DownloadSelection(ctx, client, cacheManager, opts.LicenseKey, selection, false, progress)
		if err != nil {
			// Abandon rather than Finish: Finish would paint the bar at 100%
			// and report a failed transfer as complete.
			if progressBar != nil {
				progressBar.Abandon()
			}

			if spinner != nil {
				spinner.Stop()
			}

			return nil, fmt.Errorf("failed to download %s: %w", selection.Product, err)
		}

		if progressBar != nil {
			progressBar.Finish()
		}

		successMsg := fmt.Sprintf("Downloaded %s %s (%s)", productName, selection.VersionString, ui.FormatBytes(entry.Metadata.Size))
		if spinner != nil {
			spinner.StopWithMessage("success", successMsg)
		} else {
			ui.PrintSuccess(successMsg)
		}

		if selection.Product == "xenforo" && opts.versionString == "" {
			opts.versionString = versionStr
		}

		cachedFiles[selection.Product] = entry
	}

	return cachedFiles, nil
}

// normalizeRuntimeStorage makes XenForo's runtime storage trees writable by
// the container's www-data user. Freshly extracted data/ and internal_data/
// files are owned by the host user, so on native Linux the container cannot
// write to them without world-writable permissions. Only the fresh install
// path calls this; upgrade and --existing trees are left untouched.
func normalizeRuntimeStorage(targetPath string) error {
	roots := []string{"data", "internal_data"}

	for _, name := range roots {
		root := filepath.Join(targetPath, name)

		if err := os.MkdirAll(root, 0o777); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", name, err)
		}

		if err := normalizeRuntimeTree(root); err != nil {
			return fmt.Errorf("failed to normalize %s permissions: %w", name, err)
		}
	}

	return nil
}

func normalizeRuntimeTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return os.Chmod(path, 0o777)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		return os.Chmod(path, 0o666)
	})
}

func configureEnvironment(opts *InitOptions) error {
	envPath := filepath.Join(opts.TargetPath, ".env")

	if _, err := environment.ReadEnvFile(envPath); err != nil {
		return fmt.Errorf(".env file not found after xf init: %w", err)
	}

	updates := map[string]string{
		"XF_INSTANCE":    opts.InstanceName,
		"XF_EMAIL":       opts.AdminEmail,
		"XF_DEBUG":       "1",
		"XF_DEVELOPMENT": "1",
	}

	if opts.SiteTitle != "" {
		updates["XF_TITLE"] = setup.InstanceTitle(opts.SiteTitle, opts.InstanceName)
	}

	if len(opts.Contexts) > 0 {
		updates["XF_CONTEXTS"] = strings.Join(opts.Contexts, ":")
	}

	updates["XF_COOKIE_PREFIX"] = opts.InstanceName + "_"
	maps.Copy(updates, opts.envResolved)

	if err := environment.WriteEnvFile(envPath, updates); err != nil {
		return fmt.Errorf("failed to write environment configuration: %w", err)
	}

	ui.PrintSubstep("Configured instance: " + opts.InstanceName)
	ui.PrintDetail("Admin email: " + opts.AdminEmail)

	return nil
}
