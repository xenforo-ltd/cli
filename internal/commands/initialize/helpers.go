package initialize

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/viper"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// printInstallFailure reports an xf:install failure with a single error line
// and a remediation hint, then returns an error that carries the child's own
// exit status, so callers propagate failure without handleError printing the
// same failure again.
//
// A cancelled context is reported as the cancellation itself, before any
// output: an interrupted install is not a failure with a retry step.
func printInstallFailure(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	ui.PrintError("xf:install failed")
	ui.PrintCause(err, viper.GetBool("verbose"))
	ui.PrintHint("Run " + ui.Command.Render("xf xf:install") + " to retry once the containers are up")

	return clierror.Passthrough(err, "xf:install failed")
}

// formatLicenseDetails returns the license label, falling back to the key when
// the license cannot be fetched or found.
func formatLicenseDetails(ctx context.Context, client *customerapi.Client, key string) string {
	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return key
	}

	for _, lic := range licenses {
		if lic.LicenseKey != key {
			continue
		}

		return downloads.LicenseLabel(lic)
	}

	return key
}

// getProductTitleMapCached returns product titles, fetching and caching them as needed.
func getProductTitleMapCached(ctx context.Context, client *customerapi.Client, opts *InitOptions) map[string]string {
	if len(opts.productTitleMap) > 0 {
		return opts.productTitleMap
	}

	opts.productTitleMap = downloads.ProductTitles(ctx, client, opts.LicenseKey)

	return opts.productTitleMap
}

// formatProductNames returns the display titles of products, joined for
// summary lines. A product without a title is shown by its ID.
func formatProductNames(products []string, titleMap map[string]string) string {
	names := make([]string, 0, len(products))
	for _, product := range products {
		name := strings.TrimSpace(titleMap[product])
		if name == "" {
			name = product
		}

		names = append(names, name)
	}

	return strings.Join(names, ", ")
}

// inferSiteTitleFromEnv returns the configured title without its instance suffix.
func inferSiteTitleFromEnv(opts *InitOptions) string {
	title := strings.TrimSpace(opts.envResolved["XF_TITLE"])
	if title == "" {
		return ""
	}

	if opts.InstanceName == "" {
		return title
	}

	suffix := fmt.Sprintf(" [%s]", opts.InstanceName)

	return strings.TrimSuffix(title, suffix)
}

// validateReviewInputs checks credentials and environment values before continuing.
func validateReviewInputs(opts *InitOptions) error {
	if err := validatePassword(opts.AdminPassword); err != nil {
		return err
	}

	if err := validateEmail(opts.AdminEmail); err != nil {
		return err
	}

	if strings.TrimSpace(opts.AdminUser) == "" {
		return errAdminUserRequired
	}

	for k, v := range opts.envResolved {
		if k == "XF_DEBUG" || k == "XF_DEVELOPMENT" {
			continue
		}

		if err := validateEnvKey(strings.TrimSpace(k)); err != nil {
			return fmt.Errorf("invalid environment key %q: %w", k, err)
		}

		if strings.Contains(v, "\n") {
			return fmt.Errorf("invalid environment value for %s: %w", k, errNewlinesNotAllowed)
		}
	}

	return nil
}

// ensureCoreFirstUnique puts XenForo core first and removes empty or duplicate IDs.
func ensureCoreFirstUnique(products []string) []string {
	seen := map[string]bool{}
	out := []string{"xenforo"}
	seen["xenforo"] = true

	for _, p := range products {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}

		seen[p] = true
		out = append(out, p)
	}

	return out
}

// downloadableLicenseOptions lists only licenses that allow downloads.
func downloadableLicenseOptions(licenses []customerapi.License) []huh.Option[string] {
	var options []huh.Option[string]

	for _, lic := range licenses {
		if lic.CanDownload {
			options = append(options, huh.NewOption(downloads.LicenseLabel(lic), lic.LicenseKey))
		}
	}

	return options
}

// promptProducts asks for add-ons and returns the selection with core first.
func promptProducts(downloadables *customerapi.LicenseDownloadables, current []string) ([]string, error) {
	var options []huh.Option[string]

	for _, d := range downloadables.Downloadables {
		if d.DownloadID == "xenforo" {
			continue
		}

		options = append(options, huh.NewOption(d.Title, d.DownloadID).Selected(slices.Contains(current, d.DownloadID)))
	}

	var picked []string
	if err := huh.NewMultiSelect[string]().
		Title("What additional products should be installed?").
		Description("XenForo core is always installed. Use ↑/↓ to move, Space to select, Enter to continue.").
		Options(options...).
		Value(&picked).Run(); err != nil {
		return nil, clierror.Prompt(err, "product selection")
	}

	return ensureCoreFirstUnique(picked), nil
}
