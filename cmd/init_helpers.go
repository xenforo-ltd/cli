package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/initflow"
)

func formatLicenseDetails(ctx context.Context, client *customerapi.Client, key string) string {
	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return key
	}

	for _, lic := range licenses {
		if lic.LicenseKey != key {
			continue
		}

		var parts []string
		if lic.SiteTitle != "" {
			parts = append(parts, lic.SiteTitle)
		}

		if lic.SiteURL != "" {
			parts = append(parts, lic.SiteURL)
		}

		if len(parts) == 0 && lic.ProductTitle != "" {
			parts = append(parts, lic.ProductTitle)
		}

		if len(parts) == 0 {
			return key
		}

		return fmt.Sprintf("%s (%s)", key, strings.Join(parts, " - "))
	}

	return key
}

func getProductTitleMap(ctx context.Context, client *customerapi.Client, licenseKey string) map[string]string {
	out := map[string]string{
		"xenforo": "XenForo",
	}

	downloadables, err := client.GetLicenseDownloadables(ctx, licenseKey)
	if err != nil {
		return out
	}

	for _, d := range downloadables.Downloadables {
		out[d.DownloadID] = d.Title
	}

	return out
}

func getProductTitleMapCached(ctx context.Context, client *customerapi.Client, opts *InitOptions) map[string]string {
	if len(opts.ProductTitleMap) > 0 {
		return opts.ProductTitleMap
	}

	opts.ProductTitleMap = getProductTitleMap(ctx, client, opts.LicenseKey)

	return opts.ProductTitleMap
}

func formatProductList(products []string, titleMap map[string]string) string {
	names := make([]string, 0, len(products))
	for _, p := range products {
		name := titleMap[p]
		if name == "" {
			name = p
		}

		names = append(names, name)
	}

	return strings.Join(names, ", ")
}

func effectiveContexts(opts *InitOptions) []string {
	if len(opts.Contexts) > 0 {
		return normalizeContexts(opts.Contexts)
	}

	return []string{"caddy", "mysql", "development", "caddy-development", "redis", "mailpit"}
}

func normalizeContexts(contexts []string) []string {
	set := map[string]bool{}

	for _, c := range contexts {
		c = strings.TrimSpace(c)
		if c != "" {
			set[c] = true
		}
	}

	if set["caddy"] && !set["caddy-development"] {
		set["caddy-development"] = true
	}

	if set["caddy-development"] && !set["caddy"] {
		set["caddy"] = true
	}

	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

func licenseOptionLabel(lic customerapi.License) string {
	label := lic.LicenseKey

	var parts []string
	if lic.SiteTitle != "" {
		parts = append(parts, lic.SiteTitle)
	}

	if lic.SiteURL != "" {
		parts = append(parts, lic.SiteURL)
	}

	if len(parts) > 0 {
		label = fmt.Sprintf("%s (%s)", label, strings.Join(parts, " - "))
	}

	return label
}

func inferSiteTitleFromEnv(opts *InitOptions) string {
	title := strings.TrimSpace(opts.EnvResolved["XF_TITLE"])
	if title == "" {
		return ""
	}

	if opts.InstanceName == "" {
		return title
	}

	suffix := fmt.Sprintf(" [%s]", opts.InstanceName)

	return strings.TrimSuffix(title, suffix)
}

func clearScreen() {
	_, _ = fmt.Fprint(os.Stdout, "\033[H\033[2J")
}

func validateReviewInputs(opts *InitOptions) error {
	if strings.TrimSpace(opts.AdminPassword) == "" {
		return ErrPasswordRequired
	}

	if !strings.Contains(strings.TrimSpace(opts.AdminEmail), "@") {
		return ErrValidEmailRequired
	}

	if strings.TrimSpace(opts.AdminUser) == "" {
		return ErrAdminUserRequired
	}

	for k, v := range opts.EnvResolved {
		if k == "XF_DEBUG" || k == "XF_DEVELOPMENT" {
			continue
		}

		if err := initflow.ValidateEnvKey(strings.TrimSpace(k)); err != nil {
			return fmt.Errorf("invalid environment key %q: %w", k, err)
		}

		if strings.Contains(v, "\n") {
			return fmt.Errorf("invalid environment value for %s: %w", k, initflow.ErrNewlinesNotAllowed)
		}
	}

	return nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")

	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

func ensureCoreFirstUnique(products []string) []string {
	seen := map[string]bool{}
	out := []string{"xenforo"}
	seen["xenforo"] = true

	for _, p := range products {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}

		if p == "xenforo" {
			continue
		}

		seen[p] = true
		out = append(out, p)
	}

	return out
}

func fallbackBoardURL(instanceName string) string {
	return fmt.Sprintf("https://%s.xf.local", instanceName)
}

func chooseBoardURL(instanceName, detectedURL string, detectedErr error) (string, bool) {
	if detectedErr != nil || strings.TrimSpace(detectedURL) == "" {
		return fallbackBoardURL(instanceName), false
	}

	return detectedURL, true
}

func shellJoinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t\"\\") && !strings.Contains(arg, "$(") {
			parts[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		} else {
			parts[i] = arg
		}
	}

	return strings.Join(parts, " ")
}
