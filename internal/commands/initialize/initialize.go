package initialize

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/huh/v2"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/setup"
)

// InitOptions contains options for initialization.
type InitOptions struct {
	TargetPath       string
	LicenseKey       string
	Products         []string
	VersionID        int
	versionString    string
	AdminUser        string
	AdminPassword    string
	AdminEmail       string
	SiteTitle        string
	InstanceName     string
	SkipUp           bool
	SkipInstall      bool
	SkipComposer     bool
	SkipBuildJS      bool
	ExistingOnly     bool
	Contexts         []string
	StartContainers  bool
	EnvFile          string
	EnvFlags         []string
	envResolved      map[string]string
	productOverrides map[string]int
	coreVersions     []customerapi.Version
	productTitleMap  map[string]string
}

func Run(ctx context.Context, opts *InitOptions) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	absPath, err := filepath.Abs(opts.TargetPath)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}

	opts.TargetPath = absPath
	opts.productOverrides = map[string]int{}
	opts.productTitleMap = map[string]string{}

	fileEnv := map[string]string{}
	if opts.EnvFile != "" {
		fileEnv, err = parseEnvFile(opts.EnvFile)
		if err != nil {
			return fmt.Errorf("failed to parse --env-file: %w", err)
		}
	}

	flagEnv, err := parseEnvFlags(opts.EnvFlags)
	if err != nil {
		return fmt.Errorf("failed to parse --env: %w", err)
	}

	opts.envResolved = mergeEnvMaps(map[string]string{}, fileEnv, flagEnv)

	hasXenForo, err := detectXenForo(absPath)
	if err != nil {
		return err
	}

	if hasXenForo || opts.ExistingOnly {
		shared := &setup.Options{
			TargetPath: opts.TargetPath, InstanceName: opts.InstanceName, SiteTitle: opts.SiteTitle,
			AdminUser: opts.AdminUser, AdminPassword: opts.AdminPassword, AdminEmail: opts.AdminEmail,
			Contexts: opts.Contexts, EnvResolved: opts.envResolved, StartContainers: opts.StartContainers,
			SkipComposer: opts.SkipComposer, SkipBuildJS: opts.SkipBuildJS, SkipInstall: opts.SkipInstall,
		}
		err := setup.Existing(ctx, shared)
		opts.InstanceName, opts.SiteTitle = shared.InstanceName, shared.SiteTitle
		return err
	}

	if err := setup.CheckPrerequisites(ctx); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.NoInteraction {
		err = validateNonInteractiveFlags(opts)
	} else {
		err = runInteractiveSetup(ctx, opts)
	}

	if err != nil {
		return err
	}

	return executeInit(ctx, opts)
}

func detectXenForo(path string) (bool, error) {
	xfPath := filepath.Join(path, "src", "XF.php")

	_, err := os.Stat(xfPath)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("failed to check XenForo path: %w", err)
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
		return clierror.Usage(fmt.Errorf("missing required flags in non-interactive mode: %s: %w", strings.Join(missing, ", "), clierror.ErrInvalidInput))
	}

	if len(opts.Products) == 0 {
		opts.Products = []string{"xenforo"}
	} else {
		opts.Products = ensureCoreFirstUnique(opts.Products)
	}

	return nil
}

func selectLicense(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch licenses: %w", err)
	}

	if len(licenses) == 0 {
		return clierror.MarkAs(clierror.ErrNotFound, "no licenses found for your account")
	}

	licenseOptions := downloadableLicenseOptions(licenses)

	if len(licenseOptions) == 0 {
		return clierror.MarkAs(clierror.ErrForbidden, "no licenses with download access found")
	}

	if err := huh.NewSelect[string]().
		Title("Select a license").
		Options(licenseOptions...).
		Value(&opts.LicenseKey).
		Run(); err != nil {
		return clierror.Prompt(err, "license selection")
	}

	return nil
}

func promptAdminCredentials(opts *InitOptions) error {
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
				Validate(validateAdminUsername),
			huh.NewInput().
				Title("Admin password").
				Value(&opts.AdminPassword).
				EchoMode(huh.EchoModePassword).
				Validate(validatePassword),
			huh.NewInput().
				Title("Admin email").
				Value(&opts.AdminEmail).
				Validate(validateEmail),
		),
	)

	if err := form.Run(); err != nil {
		return clierror.Prompt(err, "credential input")
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
		if err := selectLicense(ctx, client, opts); err != nil {
			return err
		}
	}

	if len(opts.Products) == 0 {
		downloadables, err := client.GetLicenseDownloadables(ctx, opts.LicenseKey)
		if err != nil {
			return fmt.Errorf("failed to fetch available downloads for license %s: %w", opts.LicenseKey, err)
		}

		products, err := promptProducts(downloadables, nil)
		if err != nil {
			return err
		}

		opts.Products = products
	}

	if len(opts.Products) > 0 {
		opts.Products = ensureCoreFirstUnique(opts.Products)
	}

	versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, "xenforo")
	if err != nil {
		return fmt.Errorf("failed to fetch XenForo versions for license %s: %w", opts.LicenseKey, err)
	}

	if len(versions.Versions) == 0 {
		return clierror.MarkAs(clierror.ErrNotFound, "no versions available")
	}

	downloads.SortVersions(versions.Versions)
	opts.coreVersions = versions.Versions

	if opts.VersionID == 0 {
		if err := chooseCoreVersionInteractively(opts); err != nil {
			return err
		}
	}

	if opts.versionString == "" {
		for _, v := range opts.coreVersions {
			if v.VersionID == opts.VersionID {
				opts.versionString = v.VersionStr
				break
			}
		}
	}

	if opts.VersionID == 0 {
		return clierror.MarkAs(clierror.ErrInvalidInput, "core version is required")
	}

	if opts.AdminUser == "" || opts.AdminPassword == "" || opts.AdminEmail == "" {
		if err := promptAdminCredentials(opts); err != nil {
			return err
		}
	}

	if opts.InstanceName == "" {
		opts.InstanceName = environment.GenerateInstanceName(filepath.Base(opts.TargetPath))
	}

	return runInteractiveReview(ctx, client, opts)
}
