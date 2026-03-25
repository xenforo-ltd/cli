package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/initflow"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func chooseCoreVersionInteractively(opts *InitOptions) error {
	display := initflow.BuildVersionOptions(opts.CoreVersions, 10)

	versionOptions := make([]huh.Option[int], 0, len(display)+1)
	for _, d := range display {
		versionOptions = append(versionOptions, huh.NewOption(d.Label, d.Value))
	}

	const manual = -1

	versionOptions = append(versionOptions, huh.NewOption(ui.Dim.Render("Enter a specific version..."), manual))

	selection := 0
	if len(versionOptions) > 0 {
		selection = versionOptions[0].Value
	}

	if err := huh.NewSelect[int]().
		Title("Select XenForo version").
		Description("Showing latest 10 versions. Choose manual entry for older versions.").
		Options(versionOptions...).
		Value(&selection).
		Run(); err != nil {
		return clierrors.Wrap(clierrors.CodeInvalidInput, "version selection cancelled", err)
	}

	if selection == manual {
		for {
			var manualInput string
			if err := huh.NewInput().
				Title("Enter XenForo version string or ID").
				Description("Examples: 2.3.9, v2.3.9, 2030900").
				Value(&manualInput).
				Run(); err != nil {
				return clierrors.Wrap(clierrors.CodeInvalidInput, "version input cancelled", err)
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
	for {
		clearScreen()
		ui.Println()
		ui.Println(ui.Bold.Render("Review configuration"))
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
			Title("Choose an action").
			Options(options...).
			Value(&choice).
			Run(); err != nil {
			return clierrors.Wrap(clierrors.CodeInvalidInput, "review cancelled", err)
		}

		switch choice {
		case "continue":
			if err := validateReviewInputs(opts); err != nil {
				ui.PrintWarning(err.Error())
				continue
			}

			return nil
		case "cancel":
			return clierrors.New(clierrors.CodeInvalidInput, "initialization cancelled")
		case "core":
			clearScreen()

			if err := editCoreSetup(ctx, client, opts); err != nil {
				return err
			}
		case "admin-site":
			clearScreen()

			if err := editAdminSite(opts); err != nil {
				return err
			}
		case "addon-overrides":
			clearScreen()

			if err := editAddonOverrides(ctx, client, opts); err != nil {
				return err
			}
		case "env":
			clearScreen()

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
		ui.KV("Admin", fmt.Sprintf("%s / %s", opts.AdminUser, opts.AdminEmail)),
		ui.KV("Instance", opts.InstanceName),
	})

	selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.VersionString, opts.ProductOverrides, nil)
	if err == nil {
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
		return err
	}

	if len(versions.Versions) == 0 {
		return clierrors.New(clierrors.CodeAPINotFound, "no versions available for this license")
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
				if strings.TrimSpace(s) == "" {
					return ErrAdminUserRequired
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
					return ErrValidEmailRequired
				}

				return nil
			}),
			huh.NewInput().Title("Instance name").Value(&opts.InstanceName),
		),
	)
	if err := form.Run(); err != nil {
		return clierrors.Wrap(clierrors.CodeInvalidInput, "admin/site edit cancelled", err)
	}

	return nil
}

func editLicense(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return err
	}

	var options []huh.Option[string]

	for _, lic := range licenses {
		if !lic.CanDownload {
			continue
		}

		label := licenseOptionLabel(lic)
		options = append(options, huh.NewOption(label, lic.LicenseKey))
	}

	if len(options) == 0 {
		return clierrors.New(clierrors.CodeAPIForbidden, "no licenses with download access found")
	}

	if err := huh.NewSelect[string]().Title("Select a license").Options(options...).Value(&opts.LicenseKey).Run(); err != nil {
		return clierrors.Wrap(clierrors.CodeInvalidInput, "license selection cancelled", err)
	}

	opts.CoreVersions = nil
	opts.ProductTitleMap = nil

	return nil
}

func editProducts(ctx context.Context, client *customerapi.Client, opts *InitOptions) error {
	downloadables, err := client.GetLicenseDownloadables(ctx, opts.LicenseKey)
	if err != nil {
		return err
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
		return err
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

		addonOptions := make([]huh.Option[string], 0, len(addons)+1)
		for _, p := range addons {
			name := titleMap[p]
			if name == "" {
				name = p
			}

			label := name
			if id, ok := opts.ProductOverrides[p]; ok {
				label = fmt.Sprintf("%s (override: %d)", name, id)
			}

			addonOptions = append(addonOptions, huh.NewOption(label, p))
		}

		addonOptions = append(addonOptions, huh.NewOption("Done", "__done__"))

		product := "__done__"
		if err := huh.NewSelect[string]().
			Title("Select add-on override to edit").
			Options(addonOptions...).
			Value(&product).Run(); err != nil {
			return err
		}

		if product == "__done__" {
			return nil
		}

		mode := "inferred"
		if _, ok := opts.ProductOverrides[product]; ok {
			mode = "override"
		}

		currentVersion := "auto"

		if selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.VersionString, opts.ProductOverrides, nil); err == nil {
			for _, s := range selections {
				if s.Product == product && strings.TrimSpace(s.VersionString) != "" {
					currentVersion = s.VersionString
					break
				}
			}
		}

		if err := huh.NewSelect[string]().
			Title("Choose override mode").
			Options(
				huh.NewOption(fmt.Sprintf("Use current version [%s]", currentVersion), "inferred"),
				huh.NewOption("Set specific version", "override"),
			).
			Value(&mode).Run(); err != nil {
			return err
		}

		if mode == "inferred" {
			delete(opts.ProductOverrides, product)
			continue
		}

		versions, err := client.GetLicenseVersions(ctx, opts.LicenseKey, product)
		if err != nil {
			return err
		}

		if len(versions.Versions) == 0 {
			return clierrors.Newf(clierrors.CodeAPINotFound, "no versions available for %s", product)
		}

		initflow.SortVersionsDesc(versions.Versions)
		optsList := initflow.BuildVersionOptions(versions.Versions, 10)

		selectOptions := make([]huh.Option[int], 0, len(optsList)+1)
		for _, d := range optsList {
			selectOptions = append(selectOptions, huh.NewOption(d.Label, d.Value))
		}

		const manual = -1

		selectOptions = append(selectOptions, huh.NewOption(ui.Dim.Render("Enter a specific version..."), manual))

		choice := selectOptions[0].Value
		if err := huh.NewSelect[int]().
			Title(fmt.Sprintf("Select version for %s", product)).
			Description("Showing latest 10 versions. Choose manual entry for older versions.").
			Options(selectOptions...).
			Value(&choice).Run(); err != nil {
			return err
		}

		if choice == manual {
			for {
				var input string
				if err := huh.NewInput().
					Title("Enter version string or version ID").
					Value(&input).Run(); err != nil {
					return err
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
	for {
		envVals, _ := currentEnvPreview(opts)

		keys := make([]string, 0, len(envVals)+2)
		for k := range envVals {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		options := make([]huh.Option[string], 0, len(keys)+2)
		for _, k := range keys {
			options = append(options, huh.NewOption(fmt.Sprintf("%s=%s", k, envVals[k]), k))
		}

		options = append(options, huh.NewOption("Add new variable", "__add__"))
		options = append(options, huh.NewOption("Done", "__done__"))

		choice := "__done__"
		if len(options) > 0 {
			choice = options[0].Value
		}

		if err := huh.NewSelect[string]().
			Title("Edit environment values").
			Options(options...).
			Value(&choice).Run(); err != nil {
			return err
		}

		if choice == "__done__" {
			return nil
		}

		key := choice
		if choice == "__add__" {
			if err := huh.NewInput().Title("Environment key").Value(&key).Run(); err != nil {
				return err
			}

			key = strings.TrimSpace(strings.ToUpper(key))
			if err := initflow.ValidateEnvKey(key); err != nil {
				ui.PrintWarning(err.Error())
				continue
			}
		}

		value := envVals[key]
		if err := huh.NewInput().Title(fmt.Sprintf("Value for %s", key)).Value(&value).Run(); err != nil {
			return err
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

func currentEnvPreview(opts *InitOptions) (map[string]string, map[string]string) {
	base := map[string]string{
		"XF_INSTANCE": opts.InstanceName,
		"XF_EMAIL":    opts.AdminEmail,
		"PHP_VERSION": "8.5",
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
		sources[k] = "inferred"
	}

	for k, v := range opts.EnvResolved {
		merged[k] = v

		src := opts.EnvSources[k]
		if src == "" {
			src = "override"
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
