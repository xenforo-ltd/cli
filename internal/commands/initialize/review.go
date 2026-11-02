package initialize

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"charm.land/huh/v2"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/setup"
	"github.com/xenforo-ltd/cli/internal/ui"
)

const (
	reviewDone   = "__done__"
	versionCount = 10

	manualVersionLabel = "Enter a specific version..."
)

// sentence upper-cases the first rune of s, leaving the rest untouched.
// Validation sentinel errors are lower-case by Go convention; this makes
// them read as sentence-case UI copy when surfaced via pendingWarning.
func sentence(s string) string {
	if s == "" {
		return s
	}

	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])

	return string(r)
}

type overrideMode int

const (
	modeInferred overrideMode = iota
	modeOverride
)

func chooseCoreVersionInteractively(opts *InitOptions) error {
	v, err := promptVersion(opts.coreVersions, versionPrompt{
		selectTitle: "Select XenForo version",
		inputTitle:  "Enter XenForo version string or ID",
		notFound:    "Version not found for this license. Try another version.",
	})
	if err != nil {
		return err
	}

	opts.VersionID = v.VersionID
	opts.versionString = v.VersionStr

	return nil
}

type versionPrompt struct {
	selectTitle string
	inputTitle  string
	notFound    string
	errSuffix   string
}

func promptVersion(versions []customerapi.Version, p versionPrompt) (customerapi.Version, error) {
	display := buildVersionOptions(versions, versionCount)

	versionOptions := make([]huh.Option[int], 0, len(display)+1)
	for _, d := range display {
		versionOptions = append(versionOptions, huh.NewOption(d.label, d.value))
	}

	const manual = -1

	versionOptions = append(versionOptions, huh.NewOption(manualVersionLabel, manual))

	selection := versionOptions[0].Value
	if err := huh.NewSelect[int]().
		Title(p.selectTitle).
		Description(fmt.Sprintf("Showing latest %d versions. Choose manual entry for older versions.", versionCount)).
		Options(versionOptions...).
		Value(&selection).
		Run(); err != nil {
		return customerapi.Version{}, clierror.Prompt(err, "version selection%s", p.errSuffix)
	}

	if selection != manual {
		for _, v := range versions {
			if v.VersionID == selection {
				return v, nil
			}
		}

		return customerapi.Version{VersionID: selection}, nil
	}

	for {
		var input string
		if err := huh.NewInput().
			Title(p.inputTitle).
			Description("Examples: 2.3.9, v2.3.9, 2030900").
			Value(&input).
			Run(); err != nil {
			return customerapi.Version{}, clierror.Prompt(err, "version input%s", p.errSuffix)
		}

		if v, ok := resolveVersionInput(input, versions); ok {
			return v, nil
		}

		ui.PrintWarning(p.notFound)
	}
}

func runInteractiveReview(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	var pendingWarning string

	for {
		ui.ClearScreen()
		ui.Println(ui.Header.Render("Review configuration"))

		if pendingWarning != "" {
			ui.PrintWarning(pendingWarning)
			ui.Println()
			pendingWarning = ""
		}

		printReviewSummary(ctx, client, opts)
		ui.Println()

		choice := "continue"

		options := []huh.Option[string]{
			huh.NewOption("Continue", "continue"),
			huh.NewOption("Edit core setup (license, products, version)", "core"),
			huh.NewOption("Edit admin/site settings", "admin-site"),
			huh.NewOption("Edit add-on version overrides", "addon-overrides"),
			huh.NewOption("Edit environment values", "env"),
			huh.NewOption("Cancel", "cancel"),
		}
		if err := huh.NewSelect[string]().
			Options(options...).
			Value(&choice).
			Run(); err != nil {
			return clierror.Prompt(err, "review")
		}

		switch choice {
		case "continue":
			if err := validateReviewInputs(opts); err != nil {
				pendingWarning = sentence(err.Error())
				continue
			}

			return nil
		case "cancel":
			return clierror.MarkAs(clierror.ErrCancelled, "initialization cancelled")
		case "core":
			ui.ClearScreen()

			if err := editCoreSetup(ctx, client, opts); err != nil {
				return err
			}
		case "admin-site":
			ui.ClearScreen()

			if err := editAdminSite(opts); err != nil {
				return err
			}
		case "addon-overrides":
			ui.ClearScreen()

			if err := editAddonOverrides(ctx, client, opts); err != nil {
				return err
			}
		case "env":
			ui.ClearScreen()

			if err := editEnvValues(opts); err != nil {
				return err
			}
		}
	}
}

func printReviewSummary(ctx context.Context, client *customerapi.Client, opts *InitOptions) {
	licenseDetails := formatLicenseDetails(ctx, client, opts.LicenseKey)
	titleMap := getProductTitleMapCached(ctx, client, opts)

	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("License", licenseDetails),
		ui.KV("Core version", opts.versionString),
		ui.KV("Products", formatProductList(opts.Products, titleMap)),
		ui.KV("Admin user", opts.AdminUser),
		ui.KV("Admin email", opts.AdminEmail),
		ui.KV("Instance", opts.InstanceName),
	})

	selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.versionString, opts.productOverrides, nil)
	if err != nil {
		ui.Println()
		ui.PrintWarning("Could not resolve add-on versions; continuing may fail during initialization")
	} else {
		ui.Println()
		ui.Println(ui.Bold.Render("Add-on versions"))

		pairs := make([]ui.KVPair, 0, len(selections))
		for _, s := range selections {
			if s.Product == "xenforo" {
				continue
			}

			label := s.VersionString
			if strings.HasPrefix(s.Reason, "latest") {
				label += " " + ui.Dim.Render("(latest)")
			}

			pairs = append(pairs, ui.KV(downloads.ProductTitle(titleMap, s.Product), label))
		}

		if len(pairs) == 0 {
			ui.Printf("%s%s\n", ui.Indent1, ui.Dim.Render("None"))
		} else {
			ui.PrintKeyValuePaddedWithIndent(pairs, ui.Indent1)
		}
	}

	envVals := currentEnvPreview(opts)

	keys := slices.Sorted(maps.Keys(envVals))

	if len(keys) > 0 {
		ui.Println()
		ui.Println(ui.Bold.Render("Environment values"))

		pairs := make([]ui.KVPair, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, ui.KV(k, envVals[k]))
		}

		ui.PrintKeyValuePaddedWithIndent(pairs, ui.Indent1)
	}
}

func editCoreSetup(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	if err := editLicense(ctx, client, opts); err != nil {
		return err
	}

	if err := editProducts(ctx, client, opts); err != nil {
		return err
	}

	versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, "xenforo")
	if err != nil {
		return fmt.Errorf("failed to fetch XenForo versions for license %s: %w", opts.LicenseKey, err)
	}

	if len(versions.Versions) == 0 {
		return fmt.Errorf("no versions available for this license: %w", clierror.ErrNotFound)
	}

	sortVersionsDesc(versions.Versions)

	opts.coreVersions = versions.Versions

	return chooseCoreVersionInteractively(opts)
}

func editAdminSite(opts *InitOptions) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Admin username").Value(&opts.AdminUser).Validate(validateAdminUsername),
			huh.NewInput().Title("Admin password").EchoMode(huh.EchoModePassword).Value(&opts.AdminPassword).Validate(validatePassword),
			huh.NewInput().Title("Admin email").Value(&opts.AdminEmail).Validate(validateEmail),
			huh.NewInput().Title("Instance name").Value(&opts.InstanceName),
		),
	)
	if err := form.Run(); err != nil {
		return clierror.Prompt(err, "admin/site edit")
	}

	return nil
}

func editLicense(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch licenses: %w", err)
	}

	options := downloadableLicenseOptions(licenses)

	if len(options) == 0 {
		return fmt.Errorf("no licenses with download access found: %w", clierror.ErrForbidden)
	}

	if err := huh.NewSelect[string]().Title("Select a license").Options(options...).Value(&opts.LicenseKey).Run(); err != nil {
		return clierror.Prompt(err, "license selection")
	}

	opts.coreVersions = nil
	opts.productTitleMap = nil

	return nil
}

func editProducts(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	downloadables, err := client.GetLicenseDownloadables(ctx, opts.LicenseKey)
	if err != nil {
		return fmt.Errorf("failed to fetch available downloads for license %s: %w", opts.LicenseKey, err)
	}

	products, err := promptProducts(downloadables, opts.Products)
	if err != nil {
		return err
	}

	opts.Products = products

	return nil
}

func editAddonOverrides(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	addons := make([]string, 0, len(opts.Products))
	for _, p := range opts.Products {
		if p != "xenforo" {
			addons = append(addons, p)
		}
	}

	if len(addons) == 0 {
		ui.PrintWarning("No additional products selected")
		return nil
	}

	for {
		titleMap := getProductTitleMapCached(ctx, client, opts)

		overrideVersions := map[string]string{}
		if selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.versionString, opts.productOverrides, nil); err == nil {
			for _, s := range selections {
				if strings.TrimSpace(s.VersionString) != "" {
					overrideVersions[s.Product] = s.VersionString
				}
			}
		}

		addonOptions := make([]huh.Option[string], 0, len(addons)+1)
		for _, p := range addons {
			name := downloads.ProductTitle(titleMap, p)

			label := name
			if id, ok := opts.productOverrides[p]; ok {
				if v, ok := overrideVersions[p]; ok {
					label = fmt.Sprintf("%s (override: %s)", name, v)
				} else {
					label = fmt.Sprintf("%s (override id %d)", name, id)
				}
			}

			addonOptions = append(addonOptions, huh.NewOption(label, p))
		}

		addonOptions = append(addonOptions, huh.NewOption("Done", reviewDone))

		product := reviewDone
		if err := huh.NewSelect[string]().
			Title("Select add-on override to edit").
			Options(addonOptions...).
			Value(&product).Run(); err != nil {
			return clierror.Prompt(err, "add-on override selection")
		}

		if product == reviewDone {
			return nil
		}

		mode := modeInferred
		if _, ok := opts.productOverrides[product]; ok {
			mode = modeOverride
		}

		currentVersion := "auto"
		if v, ok := overrideVersions[product]; ok {
			currentVersion = v
		}

		if err := huh.NewSelect[overrideMode]().
			Title("Choose override mode").
			Options(
				huh.NewOption(fmt.Sprintf("Use current version (%s)", currentVersion), modeInferred),
				huh.NewOption("Set specific version", modeOverride),
			).
			Value(&mode).Run(); err != nil {
			return clierror.Prompt(err, "override mode selection for %s", product)
		}

		if mode == modeInferred {
			delete(opts.productOverrides, product)
			continue
		}

		if err := chooseOverrideVersion(ctx, client, opts, product); err != nil {
			return err
		}
	}
}

func chooseOverrideVersion(ctx context.Context, client *customerapi.Client, opts *InitOptions, product string) error {
	versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, product)
	if err != nil {
		return fmt.Errorf("failed to fetch versions for %s: %w", product, err)
	}

	if len(versions.Versions) == 0 {
		return fmt.Errorf("no versions available for %s: %w", product, clierror.ErrNotFound)
	}

	sortVersionsDesc(versions.Versions)

	v, err := promptVersion(versions.Versions, versionPrompt{
		selectTitle: "Select version for " + product,
		inputTitle:  "Enter version string or version ID",
		notFound:    "Version not found. Try another version.",
		errSuffix:   " for " + product,
	})
	if err != nil {
		return err
	}

	opts.productOverrides[product] = v.VersionID

	return nil
}

func editEnvValues(opts *InitOptions) error {
	for {
		envVals := currentEnvPreview(opts)
		keys := slices.Sorted(maps.Keys(envVals))

		keyWidth := 0
		for _, k := range keys {
			keyWidth = max(keyWidth, len(k))
		}

		options := make([]huh.Option[string], 0, len(keys)+2)
		for _, k := range keys {
			options = append(options, huh.NewOption(fmt.Sprintf("%-*s %s", keyWidth, k, envVals[k]), k))
		}

		options = append(options, huh.NewOption("── Add new variable", "__add__"))
		options = append(options, huh.NewOption("── Done", reviewDone))

		choice := options[0].Value
		if err := huh.NewSelect[string]().
			Title("Edit environment values").
			Options(options...).
			Value(&choice).Run(); err != nil {
			return clierror.Prompt(err, "environment variable selection")
		}

		if choice == reviewDone {
			return nil
		}

		key := choice
		if choice == "__add__" {
			if err := huh.NewInput().Title("Environment key").Value(&key).Run(); err != nil {
				return clierror.Prompt(err, "environment key entry")
			}

			key = strings.TrimSpace(strings.ToUpper(key))
			if err := validateEnvKey(key); err != nil {
				ui.PrintWarning(sentence(err.Error()))
				continue
			}
		}

		value := envVals[key]
		if err := huh.NewInput().Title("Value for " + key).Value(&value).Run(); err != nil {
			return clierror.Prompt(err, "environment value entry for %s", key)
		}

		if opts.envResolved == nil {
			opts.envResolved = map[string]string{}
		}

		opts.envResolved[key] = value
	}
}

// defaultPHPVersionFallback is used only if the embedded .env.default
// template cannot be read or no longer documents a default PHP_VERSION;
// this should not happen outside of a corrupted binary.
const defaultPHPVersionFallback = "8.5"

// defaultPHPVersion reads the default PHP_VERSION from the same embedded
// .env.default template that `xf init` writes, so this preview never drifts
// from the value shipped in the Docker files.
// The line is commented out there (`#PHP_VERSION=8.5`) because Docker's
// own default (compose.yaml's `${PHP_VERSION:-8.5}`) takes over when unset.
func defaultPHPVersion() string {
	data, err := environment.GetDockerFile(".env.default")
	if err != nil {
		return defaultPHPVersionFallback
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		if v, ok := strings.CutPrefix(line, "PHP_VERSION="); ok {
			return strings.TrimSpace(v)
		}
	}

	return defaultPHPVersionFallback
}

func currentEnvPreview(opts *InitOptions) map[string]string {
	base := map[string]string{
		"XF_INSTANCE": opts.InstanceName,
		"XF_EMAIL":    opts.AdminEmail,
		"PHP_VERSION": defaultPHPVersion(),
	}
	base["XF_TITLE"] = setup.InstanceTitle(opts.SiteTitle, opts.InstanceName)

	merged := map[string]string{}

	maps.Copy(merged, base)

	maps.Copy(merged, opts.envResolved)

	delete(merged, "XF_CONTEXTS")

	if strings.TrimSpace(merged["XF_DEBUG"]) == "" || strings.TrimSpace(merged["XF_DEBUG"]) == "1" {
		delete(merged, "XF_DEBUG")
	}

	if strings.TrimSpace(merged["XF_DEVELOPMENT"]) == "" || strings.TrimSpace(merged["XF_DEVELOPMENT"]) == "1" {
		delete(merged, "XF_DEVELOPMENT")
	}

	return merged
}
