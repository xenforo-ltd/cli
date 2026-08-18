package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/initflow"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// printSkippedStep prints a step line for a step that occupies its slot in
// the plan but did not run, with its real label and a dim skip reason.
func printSkippedStep(current, total int, label, reason string) {
	ui.Printf("%s %s %s\n", ui.Step(current, total), ui.Bold.Render(label),
		ui.Dim.Render("(skipped: "+reason+")"))
}

// printInstallFailure reports an xf:install failure with a single error line
// and a remediation hint, then returns an error that carries the child's own
// exit status, so callers propagate failure without handleError printing the
// same failure again.
func printInstallFailure(err error) error {
	ui.PrintError("xf:install failed")
	ui.PrintHint("Run " + ui.Command.Render("xf xf:install") + " to retry once the containers are up")

	return passthroughError(err, "xf:install failed")
}

// printStartHint prints the hint for starting an environment that was not
// brought up during init.
func printStartHint(dir string) {
	ui.PrintHint("Run " + ui.Command.Render("xf up") + " in " + ui.Path.Render(ui.ShortHome(dir)) + " to start the environment")
}

func formatLicenseDetails(ctx context.Context, client *customerapi.Client, key string) string {
	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return key
	}

	for _, lic := range licenses {
		if lic.LicenseKey != key {
			continue
		}

		return licenseLabel(lic)
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

// licenseLabel formats a license for display: the key alone, or the key with
// its site title/URL (falling back to the product title) in parentheses.
// The result is intentionally unstyled — huh restyles select options itself.
func licenseLabel(lic customerapi.License) string {
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
		return lic.LicenseKey
	}

	return fmt.Sprintf("%s (%s)", lic.LicenseKey, strings.Join(parts, " - "))
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

func validateReviewInputs(opts *InitOptions) error {
	if strings.TrimSpace(opts.AdminPassword) == "" {
		return ErrPasswordRequired
	}

	if !strings.Contains(strings.TrimSpace(opts.AdminEmail), "@") {
		return ErrInvalidEmail
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

// installShellCommand builds the shell command that runs xf:install.
//
// Every argument is shell-quoted, so installer values such as the site title
// cannot inject shell syntax. The password substitution is quoted too, so a
// password containing spaces or glob characters reaches the installer
// verbatim.
//
// The password is passed through the environment and expanded by sh rather
// than being interpolated here. That keeps it out of xf's own argv, out of the
// docker compose invocation, and out of anything that logs either of those.
//
// It does not keep it out of the php process's argv inside the container:
// XF\Cli\Command\Install accepts the administrator password only via
// --password or an interactive hidden prompt, and --no-interaction rules the
// prompt out. So for the lifetime of the install, the password is visible to
// anything that can list processes in that container. The container is a
// single-tenant development environment created by this tool, so that exposure
// is accepted; it should be revisited if XenForo ever accepts the password on
// stdin or from an environment variable of its own.
func installShellCommand(installArgs []string) string {
	command := shellJoinArgs(append([]string{"php", "cmd.php"}, installArgs...))

	// Expanded directly rather than through $(printenv ...): command
	// substitution strips trailing newlines, so a password ending in one would
	// reach the installer altered.
	return command + ` --password="$XF_INSTALL_PASSWORD"`
}

func shellJoinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = shellQuote(arg)
	}

	return strings.Join(parts, " ")
}

// shellQuote renders a string as a single-quoted POSIX shell word.
//
// Every argument is quoted unconditionally. Quoting only those that look
// dangerous is how injection gets through: a value such as
// `--title=x;rm -rf /` contains no spaces or quotes, so a
// looks-dangerous test passes it to the shell verbatim.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
