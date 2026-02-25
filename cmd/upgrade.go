package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [path]",
	Short: "Upgrade an existing XenForo installation",
	Long: `Upgrade an existing XenForo installation to a newer version.

This command:
  1. Detects the current XenForo version
  2. Shows available upgrade versions
  3. Downloads the new version
  4. Overlays new files (preserving config and data)
  5. Runs 'xf xf:upgrade' to complete the upgrade

The target directory must contain an existing XenForo installation.
If the installation was created with 'xf init', the license
key will be detected automatically. Otherwise, use --license to specify it.

Examples:
  # Interactive upgrade (prompts for version)
  xf upgrade ./my-project

  # Upgrade to a specific version
  xf upgrade ./my-project --version 2030971

  # Non-interactive upgrade
  xf upgrade ./my-project --version 2030971 --non-interactive`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUpgrade,
}

// UpgradeOptions specifies upgrade parameters.
type UpgradeOptions struct {
	TargetPath          string
	LicenseKey          string
	TargetVersionID     int
	TargetVersionString string
	CurrentVersion      *xf.Version
	Products            []string
	SkipUpgrade         bool
}

var (
	flagUpgradeLicense string
	flagUpgradeVersion int
	flagUpgradeSkipCmd bool
)

func init() {
	upgradeCmd.Flags().StringVar(&flagUpgradeLicense, "license", "", "license key (auto-detected if not specified)")
	upgradeCmd.Flags().IntVar(&flagUpgradeVersion, "version", 0, "target version ID")
	upgradeCmd.Flags().BoolVar(&flagUpgradeSkipCmd, "skip-upgrade", false, "skip running xf:upgrade command")

	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
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

	opts := &UpgradeOptions{
		TargetPath:      absPath,
		LicenseKey:      flagUpgradeLicense,
		TargetVersionID: flagUpgradeVersion,
		SkipUpgrade:     flagUpgradeSkipCmd,
	}

	fmt.Println(ui.Bold.Render("Checking installation..."))

	currentVersion, err := xf.DetectVersion(absPath)
	if err != nil {
		return err
	}

	opts.CurrentVersion = currentVersion

	fmt.Println()
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("Current version", fmt.Sprintf("%s (ID: %d)", currentVersion.String, currentVersion.ID)),
	})

	meta, err := xf.ReadMetadata(absPath)
	if err != nil && !errors.Is(err, xf.ErrMetadataNotFound) {
		return err
	}

	if meta != nil {
		var pairs []ui.KVPair

		if opts.LicenseKey == "" {
			opts.LicenseKey = meta.LicenseKey
			pairs = append(pairs, ui.KV("License key", fmt.Sprintf("%s (from metadata)", opts.LicenseKey)))
		}

		opts.Products = meta.InstalledProducts
		if len(opts.Products) > 0 {
			pairs = append(pairs, ui.KV("Products", strings.Join(opts.Products, ", ")))
		}

		if len(pairs) > 0 {
			ui.PrintKeyValuePadded(pairs)
		}
	}

	flags := config.GetFlags()
	if !flags.NonInteractive {
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
		return clierrors.Newf(clierrors.CodeInvalidInput, "missing required flags in non-interactive mode: %s", strings.Join(missing, ", "))
	}

	if len(opts.Products) == 0 {
		opts.Products = []string{"xenforo"}
	}

	return nil
}

func runUpgradeInteractive(ctx context.Context, opts *UpgradeOptions) error {
	client, err := customerapi.NewClient()
	if err != nil {
		return err
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
				label := fmt.Sprintf("%s - %s", lic.LicenseKey, lic.ProductTitle)
				if lic.SiteTitle != "" {
					label = fmt.Sprintf("%s (%s)", label, lic.SiteTitle)
				}

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
		opts.Products = []string{"xenforo"}
	}

	if opts.TargetVersionID == 0 {
		versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, "xenforo")
		if err != nil {
			return err
		}

		if len(versions.Versions) == 0 {
			return clierrors.New(clierrors.CodeAPINotFound, "no versions available")
		}

		var versionOptions []huh.Option[int]

		for _, v := range versions.Versions {
			if v.VersionID > opts.CurrentVersion.ID {
				label := v.VersionStr
				if v.Stable {
					label += " (stable)"
				}

				versionOptions = append(versionOptions, huh.NewOption(label, v.VersionID))
			}
		}

		if len(versionOptions) == 0 {
			fmt.Println()
			ui.PrintSuccess("No newer versions available. Your installation is up to date!")

			return nil
		}

		err = huh.NewSelect[int]().
			Title("Select target version").
			Description(fmt.Sprintf("Current: %s", opts.CurrentVersion.String)).
			Options(versionOptions...).
			Value(&opts.TargetVersionID).
			Run()
		if err != nil {
			return clierrors.Wrap(clierrors.CodeInvalidInput, "version selection cancelled", err)
		}

		for _, v := range versions.Versions {
			if v.VersionID == opts.TargetVersionID {
				opts.TargetVersionString = v.VersionStr
				break
			}
		}
	}

	return nil
}

func executeUpgrade(ctx context.Context, opts *UpgradeOptions) error {
	if opts.TargetVersionID <= opts.CurrentVersion.ID {
		return clierrors.Newf(clierrors.CodeInvalidInput,
			"target version %d is not newer than current version %d",
			opts.TargetVersionID, opts.CurrentVersion.ID)
	}

	client, err := customerapi.NewClient()
	if err != nil {
		return err
	}

	totalSteps := 3

	fmt.Println()
	ui.PrintStep(1, totalSteps, "Downloading upgrade files")

	cachedFiles, err := downloadUpgradeFiles(ctx, client, opts)
	if err != nil {
		return err
	}

	fmt.Println()
	ui.PrintStep(2, totalSteps, "Upgrading files")

	if err := overlayUpgradeFiles(cachedFiles, opts.TargetPath); err != nil {
		return err
	}

	targetVersion := xf.ParseVersionID(opts.TargetVersionID)
	if opts.TargetVersionString != "" {
		targetVersion.String = opts.TargetVersionString
	}

	if err := xf.UpdateMetadataVersion(opts.TargetPath, targetVersion); err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not update metadata: %v", err))
	}

	fmt.Println()

	if !opts.SkipUpgrade {
		ui.PrintStep(3, totalSteps, "Running XenForo upgrade")

		runner, err := dockercompose.NewRunner(opts.TargetPath)
		if err != nil {
			return err
		}

		if err := runner.XFCommand("xf:upgrade"); err != nil {
			ui.PrintWarning(fmt.Sprintf("xf:upgrade failed: %v", err))
			fmt.Println("    You may need to start the containers first with 'up',")
			fmt.Println("    then run the upgrade manually:")
			fmt.Printf("    %s\n", ui.Command.Render(fmt.Sprintf("cd %s && xf xf:upgrade", opts.TargetPath)))
		}
	} else {
		ui.PrintStep(3, totalSteps, "Skipped (--skip-upgrade flag set)")
	}

	fmt.Println()
	ui.SuccessBox("XenForo upgrade completed!", []ui.KVPair{
		ui.KV("Location", ui.Path.Render(opts.TargetPath)),
		ui.KV("Previous version", opts.CurrentVersion.String),
		ui.KV("New version", ui.Version.Render(opts.TargetVersionString)),
	})

	if !opts.SkipUpgrade {
		fmt.Println()
		ui.PrintSuccess("Your XenForo installation has been upgraded.")
	} else {
		fmt.Println()
		fmt.Println("Files have been upgraded. Run the following to complete:")
		fmt.Printf("%s%s\n", ui.Indent1, ui.Command.Render(fmt.Sprintf("cd %s", opts.TargetPath)))
		fmt.Printf("%s%s\n", ui.Indent1, ui.Command.Render("xf up"))
		fmt.Printf("%s%s\n", ui.Indent1, ui.Command.Render("xf xf:upgrade"))
	}

	return nil
}

func downloadUpgradeFiles(ctx context.Context, client *customerapi.Client, opts *UpgradeOptions) (map[string]*cache.Entry, error) {
	cacheManager, err := cache.NewManager()
	if err != nil {
		return nil, err
	}

	cachedFiles := make(map[string]*cache.Entry)

	selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.TargetVersionID, opts.TargetVersionString, nil, func(product string) {
		ui.PrintWarning(fmt.Sprintf("No versions available for %s, skipping", product))
	})
	if err != nil {
		return nil, err
	}

	for _, selection := range selections {
		ui.PrintSubstep(fmt.Sprintf("Downloading %s...", selection.Product))

		entry, versionStr, err := downloads.DownloadSelection(ctx, client, cacheManager, opts.LicenseKey, selection, false, nil)
		if err != nil {
			return nil, err
		}

		if selection.Product == "xenforo" && opts.TargetVersionString == "" {
			opts.TargetVersionString = versionStr
		}

		ui.PrintDetail(fmt.Sprintf("Downloaded: %s (%s)", entry.Metadata.Filename, ui.FormatBytes(entry.Metadata.Size)))
		cachedFiles[selection.Product] = entry
	}

	return cachedFiles, nil
}

func overlayUpgradeFiles(cachedFiles map[string]*cache.Entry, targetPath string) error {
	return extractCachedFiles(cachedFiles, targetPath, nil, "Updated")
}
