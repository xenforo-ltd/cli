package upgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/viper"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
)

const progressUpdateStep = 100 * 1024

// UpgradeOptions specifies upgrade parameters.
type UpgradeOptions struct {
	TargetPath          string
	LicenseKey          string
	TargetVersionID     int
	targetVersionString string
	currentVersion      *xf.Version
	products            []string
	SkipUpgrade         bool
}

func Run(ctx context.Context, opts *UpgradeOptions) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	absPath, err := filepath.Abs(opts.TargetPath)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}

	opts.TargetPath = absPath

	ui.Println(ui.Header.Render("Checking installation"))
	ui.Println()

	currentVersion, err := xf.DetectVersion(absPath)
	if err != nil {
		return fmt.Errorf("failed to detect current XenForo version: %w", err)
	}

	opts.currentVersion = currentVersion

	meta, err := xf.ReadMetadata(absPath)
	if err != nil && !errors.Is(err, xf.ErrMetadataNotFound) {
		return fmt.Errorf("failed to read installation metadata: %w", err)
	}

	pairs := []ui.KVPair{
		ui.KV("Current version", ui.Version.Render(currentVersion.String)+ui.Dim.Render(fmt.Sprintf(" (id %d)", currentVersion.ID))),
	}

	if meta != nil {
		if opts.LicenseKey == "" {
			opts.LicenseKey = meta.LicenseKey
			pairs = append(pairs, ui.KV("License key", opts.LicenseKey+ui.Dim.Render(" (from metadata)")))
		}

		opts.products = meta.InstalledProducts
		if len(opts.products) > 0 {
			pairs = append(pairs, ui.KV("Products", strings.Join(opts.products, ", ")))
		}
	}

	ui.PrintKeyValuePadded(pairs)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if !cfg.NoInteraction {
		if err := runUpgradeInteractive(ctx, opts); err != nil {
			return err
		}

		if opts.TargetVersionID == 0 {
			return nil
		}
	} else {
		if err := validateUpgradeFlags(opts); err != nil {
			return err
		}
	}

	return executeUpgrade(ctx, opts)
}

func validateUpgradeFlags(opts *UpgradeOptions) error {
	var missing []string

	if opts.LicenseKey == "" {
		missing = append(missing, "--license")
	}

	if opts.TargetVersionID == 0 {
		missing = append(missing, "--version")
	}

	if len(missing) > 0 {
		return clierror.Usage(fmt.Errorf("missing required flags in non-interactive mode: %s: %w", strings.Join(missing, ", "), clierror.ErrInvalidInput))
	}

	return nil
}

func runUpgradeInteractive(ctx context.Context, opts *UpgradeOptions) error {
	client, err := customerapi.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create customer API client: %w", err)
	}

	if opts.LicenseKey == "" {
		if err := selectLicense(ctx, client, opts); err != nil {
			return err
		}
	}

	if opts.TargetVersionID == 0 {
		return selectTargetVersion(ctx, client, opts)
	}

	return nil
}

func selectLicense(ctx context.Context, client *customerapi.Client, opts *UpgradeOptions) error {
	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch licenses: %w", err)
	}

	if len(licenses) == 0 {
		return fmt.Errorf("no licenses found for your account: %w", clierror.ErrNotFound)
	}

	var licenseOptions []huh.Option[string]

	for _, lic := range licenses {
		if lic.CanDownload {
			label := fmt.Sprintf("%s - %s", lic.LicenseKey, lic.ProductTitle)
			if lic.SiteTitle != "" {
				label = fmt.Sprintf("%s (%s)", label, lic.SiteTitle)
			}

			licenseOptions = append(licenseOptions, huh.NewOption(label, lic.LicenseKey))
		}
	}

	if len(licenseOptions) == 0 {
		return fmt.Errorf("no licenses with download access found: %w", clierror.ErrForbidden)
	}

	err = huh.NewSelect[string]().
		Title("Select a license").
		Options(licenseOptions...).
		Value(&opts.LicenseKey).
		Run()
	if err != nil {
		return clierror.Prompt(err, "license selection")
	}

	return nil
}

func selectTargetVersion(ctx context.Context, client *customerapi.Client, opts *UpgradeOptions) error {
	versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, "xenforo")
	if err != nil {
		return fmt.Errorf("failed to fetch XenForo versions for license %s: %w", opts.LicenseKey, err)
	}

	if len(versions.Versions) == 0 {
		return fmt.Errorf("no versions available: %w", clierror.ErrNotFound)
	}

	var versionOptions []huh.Option[int]

	for _, v := range versions.Versions {
		if v.VersionID > opts.currentVersion.ID {
			label := v.VersionStr
			if v.Stable {
				label += " (stable)"
			}

			versionOptions = append(versionOptions, huh.NewOption(label, v.VersionID))
		}
	}

	if len(versionOptions) == 0 {
		ui.Println()
		ui.PrintSuccess("Already on the latest version")

		return nil
	}

	err = huh.NewSelect[int]().
		Title("Select target version").
		Description("Current: " + opts.currentVersion.String).
		Options(versionOptions...).
		Value(&opts.TargetVersionID).
		Run()
	if err != nil {
		return clierror.Prompt(err, "version selection")
	}

	if i := slices.IndexFunc(versions.Versions, func(v customerapi.Version) bool { return v.VersionID == opts.TargetVersionID }); i >= 0 {
		opts.targetVersionString = versions.Versions[i].VersionStr
	}

	return nil
}

func executeUpgrade(ctx context.Context, opts *UpgradeOptions) error {
	if opts.TargetVersionID <= opts.currentVersion.ID {
		return fmt.Errorf("target version %d is not newer than current version %d: %w",
			opts.TargetVersionID, opts.currentVersion.ID, clierror.ErrInvalidInput)
	}

	client, err := customerapi.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create customer API client: %w", err)
	}

	if len(opts.products) == 0 {
		opts.products = []string{"xenforo"}
	}

	step := 1

	totalSteps := 3
	if opts.SkipUpgrade {
		totalSteps = 2
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Downloading upgrade files")
	step++

	cachedFiles, err := downloadUpgradeFiles(ctx, client, opts)
	if err != nil {
		return err
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Upgrading files")
	step++

	if err := downloads.ExtractCachedFiles(cachedFiles, opts.TargetPath, nil, "Updated"); err != nil {
		return err
	}

	targetVersion := xf.ParseVersionID(opts.TargetVersionID)
	if opts.targetVersionString != "" {
		targetVersion.String = opts.targetVersionString
	}

	if err := xf.UpdateMetadataVersion(opts.TargetPath, targetVersion); err != nil {
		ui.PrintWarning("Could not update metadata")
		ui.PrintCause(err, viper.GetBool("verbose"))
	}

	if !opts.SkipUpgrade {
		ui.Println()
		ui.PrintStep(step, totalSteps, "Running XenForo upgrade")

		runner, err := environment.NewRunner(opts.TargetPath)
		if err != nil {
			return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
		}

		if err := runner.ExecOrRun(ctx, "xf", nil, os.Stdin, os.Stdout, os.Stderr, "php", "cmd.php", "xf:upgrade"); err != nil {
			return reportUpgradeFailure(ctx, err)
		}
	}

	successDetails := []ui.KVPair{
		ui.KV("Path", ui.Path.Render(ui.ShortHome(opts.TargetPath))),
	}
	if opts.targetVersionString != "" {
		successDetails = append(successDetails, ui.KV("New version", ui.Version.Render(opts.targetVersionString)))
	}

	ui.Println()
	ui.SuccessBox("XenForo upgrade complete", successDetails)

	if opts.SkipUpgrade {
		ui.Println()
		ui.PrintHint("Complete the upgrade: " + ui.Command.Render("cd "+ui.ShortHome(opts.TargetPath)) + " && " + ui.Command.Render("xf xf:upgrade"))
	}

	return nil
}

// reportUpgradeFailure reports an xf:upgrade failure with a single error line
// and a remediation hint, then returns an error that carries the child's own
// exit status.
//
// A cancelled context is reported as the cancellation itself, before any
// output: Ctrl-C during an upgrade is not a failure with a retry step.
func reportUpgradeFailure(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	ui.PrintError("xf:upgrade failed")
	ui.PrintHint("Start the environment with " + ui.Command.Render("xf up") + " and run " + ui.Command.Render("xf xf:upgrade") + " to finish")

	return clierror.Passthrough(err, "xf:upgrade failed")
}

func downloadUpgradeFiles(ctx context.Context, client *customerapi.Client, opts *UpgradeOptions) (map[string]*cache.Entry, error) {
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

	selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.products, opts.TargetVersionID, opts.targetVersionString, nil, func(product string) {
		ui.PrintWarning("No versions available for " + productTitle(product) + "; skipping")
	})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve upgrade selections for license %s: %w", opts.LicenseKey, err)
	}

	for _, selection := range selections {
		productName := productTitle(selection.Product)

		var lastUpdate int64

		spinner := ui.NewSpinner(fmt.Sprintf("Downloading %s %s", productName, selection.VersionString))
		spinner.Start()

		progress := func(current, _ int64) {
			if current-lastUpdate < progressUpdateStep && lastUpdate != 0 {
				return
			}

			lastUpdate = current

			spinner.UpdateMessage(fmt.Sprintf("Downloading %s %s (%s)", productName, selection.VersionString, ui.FormatBytes(current)))
		}

		entry, versionStr, err := downloads.DownloadSelection(ctx, client, cacheManager, opts.LicenseKey, selection, false, progress)
		if err != nil {
			spinner.Stop()

			return nil, fmt.Errorf("failed to download %s for upgrade: %w", selection.Product, err)
		}

		if selection.Product == "xenforo" && opts.targetVersionString == "" {
			opts.targetVersionString = versionStr
		}

		spinner.StopWithMessage("success", fmt.Sprintf("Downloaded %s %s (%s)", productName, selection.VersionString, ui.FormatBytes(entry.Metadata.Size)))
		cachedFiles[selection.Product] = entry
	}

	return cachedFiles, nil
}
