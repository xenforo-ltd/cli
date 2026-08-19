package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/huh/v2"

	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/docker"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/initflow"
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

func (m overrideMode) String() string {
	switch m {
	case modeInferred:
		return "inferred"
	case modeOverride:
		return "override"
	default:
		return ""
	}
}

func chooseCoreVersionInteractively(opts *InitOptions) error {
	display := initflow.BuildVersionOptions(opts.CoreVersions, versionCount)

	versionOptions := make([]huh.Option[int], 0, len(display)+1)
	for _, d := range display {
		versionOptions = append(versionOptions, huh.NewOption(d.Label, d.Value))
	}

	const manual = -1

	versionOptions = append(versionOptions, huh.NewOption(manualVersionLabel, manual))

	selection := 0
	if len(versionOptions) > 0 {
		selection = versionOptions[0].Value
	}

	if err := huh.NewSelect[int]().
		Title("Select XenForo version").
		Description(fmt.Sprintf("Showing latest %d versions. Choose manual entry for older versions.", versionCount)).
		Options(versionOptions...).
		Value(&selection).
		Run(); err != nil {
		return markAs(ErrCancelled, "version selection cancelled")
	}

	if selection == manual {
		for {
			var manualInput string
			if err := huh.NewInput().
				Title("Enter XenForo version string or ID").
				Description("Examples: 2.3.9, v2.3.9, 2030900").
				Value(&manualInput).
				Run(); err != nil {
				return markAs(ErrCancelled, "version input cancelled")
			}

			v, ok := initflow.ResolveVersionInput(manualInput, opts.CoreVersions)
			if !ok {
				ui.PrintWarning("Version not found for this license. Try another version.")
				continue
			}

			opts.VersionID = v.VersionID
			opts.VersionString = v.VersionStr

			return nil
		}
	}

	opts.VersionID = selection
	for _, v := range opts.CoreVersions {
		if v.VersionID == selection {
			opts.VersionString = v.VersionStr
			break
		}
	}

	return nil
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
			return markAs(ErrCancelled, "review cancelled")
		}

		switch choice {
		case "continue":
			if err := validateReviewInputs(opts); err != nil {
				pendingWarning = sentence(err.Error())
				continue
			}

			return nil
		case "cancel":
			return markAs(ErrCancelled, "initialization cancelled")
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
		ui.KV("Core version", opts.VersionString),
		ui.KV("Products", formatProductList(opts.Products, titleMap)),
		ui.KV("Admin user", opts.AdminUser),
		ui.KV("Admin email", opts.AdminEmail),
		ui.KV("Instance", opts.InstanceName),
	})

	selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.VersionString, opts.ProductOverrides, nil)
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

			name := titleMap[s.Product]
			if name == "" {
				name = s.Product
			}

			pairs = append(pairs, ui.KV(name, label))
		}

		if len(pairs) == 0 {
			ui.Printf("%s%s\n", ui.Indent1, ui.Dim.Render("None"))
		} else {
			ui.PrintKeyValuePaddedWithIndent(pairs, ui.Indent1)
		}
	}

	envVals, _ := currentEnvPreview(opts)

	keys := make([]string, 0, len(envVals))
	for k := range envVals {
		keys = append(keys, k)
	}

	sort.Strings(keys)

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
		return fmt.Errorf("no versions available for this license: %w", ErrNotFound)
	}

	initflow.SortVersionsDesc(versions.Versions)

	opts.CoreVersions = versions.Versions
	if err := chooseCoreVersionInteractively(opts); err != nil {
		return err
	}

	return nil
}

func editAdminSite(opts *InitOptions) error {
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title("Admin username").Value(&opts.AdminUser).Validate(func(s string) error {
				// Counted in runes: len would measure bytes, so a single
				// multi-byte character would pass a three-character minimum.
				if utf8.RuneCountInString(s) < minimumUsernameLength {
					return ErrUsernameTooShort
				}

				return nil
			}),
			huh.NewInput().Title("Admin password").EchoMode(huh.EchoModePassword).Value(&opts.AdminPassword).Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return ErrPasswordRequired
				}

				return nil
			}),
			huh.NewInput().Title("Admin email").Value(&opts.AdminEmail).Validate(func(s string) error {
				if !strings.Contains(strings.TrimSpace(s), "@") {
					return ErrInvalidEmail
				}

				return nil
			}),
			huh.NewInput().Title("Instance name").Value(&opts.InstanceName),
		),
	)
	if err := form.Run(); err != nil {
		return markAs(ErrCancelled, "admin/site edit cancelled")
	}

	return nil
}

func editLicense(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch licenses: %w", err)
	}

	var options []huh.Option[string]

	for _, lic := range licenses {
		if !lic.CanDownload {
			continue
		}

		label := licenseLabel(lic)
		options = append(options, huh.NewOption(label, lic.LicenseKey))
	}

	if len(options) == 0 {
		return fmt.Errorf("no licenses with download access found: %w", ErrForbidden)
	}

	if err := huh.NewSelect[string]().Title("Select a license").Options(options...).Value(&opts.LicenseKey).Run(); err != nil {
		return markAs(ErrCancelled, "license selection cancelled")
	}

	opts.CoreVersions = nil
	opts.ProductTitleMap = nil

	return nil
}

func editProducts(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	downloadables, err := client.GetLicenseDownloadables(ctx, opts.LicenseKey)
	if err != nil {
		return fmt.Errorf("failed to fetch available downloads for license %s: %w", opts.LicenseKey, err)
	}

	var options []huh.Option[string]

	selected := map[string]bool{}

	for _, p := range opts.Products {
		if p != "xenforo" {
			selected[p] = true
		}
	}

	for _, d := range downloadables.Downloadables {
		if d.DownloadID == "xenforo" {
			continue
		}

		o := huh.NewOption(d.Title, d.DownloadID)
		if selected[d.DownloadID] {
			o = o.Selected(true)
		}

		options = append(options, o)
	}

	var picked []string
	if err := huh.NewMultiSelect[string]().
		Title("What additional products should be installed?").
		Description("XenForo core is always installed. Use ↑/↓ to move, Space to select, Enter to continue.").
		Options(options...).
		Value(&picked).Run(); err != nil {
		return markAs(ErrCancelled, "product selection cancelled")
	}

	opts.Products = ensureCoreFirstUnique(append([]string{"xenforo"}, picked...))

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
		if selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.VersionString, opts.ProductOverrides, nil); err == nil {
			for _, s := range selections {
				if strings.TrimSpace(s.VersionString) != "" {
					overrideVersions[s.Product] = s.VersionString
				}
			}
		}

		addonOptions := make([]huh.Option[string], 0, len(addons)+1)
		for _, p := range addons {
			name := titleMap[p]
			if name == "" {
				name = p
			}

			label := name
			if id, ok := opts.ProductOverrides[p]; ok {
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
			return markAs(ErrCancelled, "add-on override selection cancelled")
		}

		if product == reviewDone {
			return nil
		}

		mode := modeInferred
		if _, ok := opts.ProductOverrides[product]; ok {
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
			return markAs(ErrCancelled, "override mode selection cancelled for %s", product)
		}

		if mode == modeInferred {
			delete(opts.ProductOverrides, product)
			continue
		}

		versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, product)
		if err != nil {
			return fmt.Errorf("failed to fetch versions for %s: %w", product, err)
		}

		if len(versions.Versions) == 0 {
			return fmt.Errorf("no versions available for %s: %w", product, ErrNotFound)
		}

		initflow.SortVersionsDesc(versions.Versions)
		optsList := initflow.BuildVersionOptions(versions.Versions, versionCount)

		selectOptions := make([]huh.Option[int], 0, len(optsList)+1)
		for _, d := range optsList {
			selectOptions = append(selectOptions, huh.NewOption(d.Label, d.Value))
		}

		const manual = -1

		selectOptions = append(selectOptions, huh.NewOption(manualVersionLabel, manual))

		choice := selectOptions[0].Value
		if err := huh.NewSelect[int]().
			Title("Select version for " + product).
			Description(fmt.Sprintf("Showing latest %d versions. Choose manual entry for older versions.", versionCount)).
			Options(selectOptions...).
			Value(&choice).Run(); err != nil {
			return markAs(ErrCancelled, "version selection cancelled for %s", product)
		}

		if choice == manual {
			for {
				var input string
				if err := huh.NewInput().
					Title("Enter version string or version ID").
					Description("Examples: 2.3.9, v2.3.9, 2030900").
					Value(&input).Run(); err != nil {
					return markAs(ErrCancelled, "version input cancelled for %s", product)
				}

				v, ok := initflow.ResolveVersionInput(input, versions.Versions)
				if !ok {
					ui.PrintWarning("Version not found. Try another version.")
					continue
				}

				opts.ProductOverrides[product] = v.VersionID

				break
			}

			continue
		}

		opts.ProductOverrides[product] = choice
	}
}

func editEnvValues(opts *InitOptions) error {
	additionalEnvChoices := 2

	for {
		envVals, _ := currentEnvPreview(opts)

		keys := make([]string, 0, len(envVals)+additionalEnvChoices)
		for k := range envVals {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		keyWidth := 0
		for _, k := range keys {
			if len(k) > keyWidth {
				keyWidth = len(k)
			}
		}

		options := make([]huh.Option[string], 0, len(keys)+additionalEnvChoices)
		for _, k := range keys {
			options = append(options, huh.NewOption(fmt.Sprintf("%-*s %s", keyWidth, k, envVals[k]), k))
		}

		options = append(options, huh.NewOption("── Add new variable", "__add__"))
		options = append(options, huh.NewOption("── Done", reviewDone))

		choice := reviewDone
		if len(options) > 0 {
			choice = options[0].Value
		}

		if err := huh.NewSelect[string]().
			Title("Edit environment values").
			Options(options...).
			Value(&choice).Run(); err != nil {
			return markAs(ErrCancelled, "environment variable selection cancelled")
		}

		if choice == reviewDone {
			return nil
		}

		key := choice
		if choice == "__add__" {
			if err := huh.NewInput().Title("Environment key").Value(&key).Run(); err != nil {
				return markAs(ErrCancelled, "environment key entry cancelled")
			}

			key = strings.TrimSpace(strings.ToUpper(key))
			if err := initflow.ValidateEnvKey(key); err != nil {
				ui.PrintWarning(sentence(err.Error()))
				continue
			}
		}

		value := envVals[key]
		if err := huh.NewInput().Title("Value for " + key).Value(&value).Run(); err != nil {
			return markAs(ErrCancelled, "environment value entry cancelled for %s", key)
		}

		if opts.EnvResolved == nil {
			opts.EnvResolved = map[string]string{}
		}

		if opts.EnvSources == nil {
			opts.EnvSources = map[string]string{}
		}

		opts.EnvResolved[key] = value
		opts.EnvSources[key] = "review"
	}
}

// defaultPHPVersionFallback is used only if the embedded .env.default
// template cannot be read or no longer documents a default PHP_VERSION;
// this should not happen outside of a corrupted binary.
const defaultPHPVersionFallback = "8.5"

// defaultPHPVersion reads the default PHP_VERSION from the same embedded
// .env.default template that docker.GetEnvDefault serves for `xf init`,
// so this preview never drifts from the value shipped in the Docker files.
// The line is commented out there (`#PHP_VERSION=8.5`) because Docker's
// own default (compose.yaml's `${PHP_VERSION:-8.5}`) takes over when unset.
func defaultPHPVersion() string {
	data, err := docker.GetEnvDefault()
	if err != nil {
		return defaultPHPVersionFallback
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		if v, ok := strings.CutPrefix(line, "PHP_VERSION="); ok {
			return strings.TrimSpace(v)
		}
	}

	return defaultPHPVersionFallback
}

func currentEnvPreview(opts *InitOptions) (map[string]string, map[string]string) {
	base := map[string]string{
		"XF_INSTANCE": opts.InstanceName,
		"XF_EMAIL":    opts.AdminEmail,
		"PHP_VERSION": defaultPHPVersion(),
	}
	if opts.SiteTitle != "" {
		base["XF_TITLE"] = fmt.Sprintf("%s [%s]", opts.SiteTitle, opts.InstanceName)
	} else {
		base["XF_TITLE"] = fmt.Sprintf("XenForo [%s]", opts.InstanceName)
	}

	merged := map[string]string{}
	sources := map[string]string{}

	for k, v := range base {
		merged[k] = v
		sources[k] = modeInferred.String()
	}

	for k, v := range opts.EnvResolved {
		merged[k] = v

		src := opts.EnvSources[k]
		if src == "" {
			src = modeOverride.String()
		}

		sources[k] = src
	}

	delete(merged, "XF_CONTEXTS")
	delete(sources, "XF_CONTEXTS")

	if strings.TrimSpace(merged["XF_DEBUG"]) == "" || strings.TrimSpace(merged["XF_DEBUG"]) == "1" {
		delete(merged, "XF_DEBUG")
		delete(sources, "XF_DEBUG")
	}

	if strings.TrimSpace(merged["XF_DEVELOPMENT"]) == "" || strings.TrimSpace(merged["XF_DEVELOPMENT"]) == "1" {
		delete(merged, "XF_DEVELOPMENT")
		delete(sources, "XF_DEVELOPMENT")
	}

	return merged, sources
}
