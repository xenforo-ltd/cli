// Package completion provides shell completion candidates for commands that
// take license keys, download IDs, cache entries or Docker contexts.
package completion

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// timeout bounds the network calls made while completing. Shells run the
// binary on every TAB, so a slow API makes completion feel broken rather than
// slow. A timeout leaves the user with no suggestions instead.
const timeout = 1500 * time.Millisecond

// LicenseKeys lists the keys of the customer's licenses. It falls back to the
// licenses with cached downloads when the API is unavailable, which are usually
// the ones in play.
func LicenseKeys(cmd *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	client, err := customerapi.NewClient()
	if err != nil {
		return CachedLicenseKeys(cmd, nil, "")
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return CachedLicenseKeys(cmd, nil, "")
	}

	completions := make([]cobra.Completion, 0, len(licenses))

	for _, license := range licenses {
		completions = append(completions,
			cobra.CompletionWithDesc(license.LicenseKey, licenseDescription(license)))
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// licenseDescription combines the product title with the site title or URL.
func licenseDescription(license customerapi.License) string {
	site := license.SiteTitle

	if site == "" {
		site = strings.TrimPrefix(strings.TrimPrefix(license.SiteURL, "https://"), "http://")
	}

	if site == "" {
		return license.ProductTitle
	}

	return license.ProductTitle + " (" + site + ")"
}

// DownloadIDs lists the packages a license can download. It reads the --license
// flag, so the two flags complete in sequence.
func DownloadIDs(cmd *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	licenseKey, err := cmd.Flags().GetString("license")
	if err != nil || licenseKey == "" {
		return cobra.AppendActiveHelp(nil, "Set --license first to list its packages"),
			cobra.ShellCompDirectiveNoFileComp
	}

	client, err := customerapi.NewClient()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	downloadables, err := client.GetLicenseDownloadables(ctx, licenseKey)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	completions := make([]cobra.Completion, 0, len(downloadables.Downloadables))

	for _, downloadable := range downloadables.Downloadables {
		completions = append(completions,
			cobra.CompletionWithDesc(downloadable.DownloadID, downloadable.Title))
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// CachedLicenseKeys lists the licenses with cached downloads, which is the only
// set the cache commands can operate on. It reads local metadata, so it needs no
// network.
func CachedLicenseKeys(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	manager, err := cache.NewManager()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	entries, err := manager.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var keys []string

	counts := make(map[string]int, len(entries))

	for _, entry := range entries {
		if _, seen := counts[entry.LicenseKey]; !seen {
			keys = append(keys, entry.LicenseKey)
		}

		counts[entry.LicenseKey]++
	}

	completions := make([]cobra.Completion, 0, len(keys))

	for _, key := range keys {
		completions = append(completions,
			cobra.CompletionWithDesc(key, ui.Plural(counts[key], "download", "downloads")))
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// DockerContexts lists the contexts xf can enable, derived from the embedded
// compose files. Values already passed to the repeatable flag are omitted.
func DockerContexts(cmd *cobra.Command, _ []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	chosen, err := cmd.Flags().GetStringArray("context")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	names, err := contextNames()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	completions := make([]cobra.Completion, 0, len(names))

	for _, name := range names {
		if strings.HasPrefix(name, toComplete) && !slices.Contains(chosen, name) {
			completions = append(completions, name)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// contextNames returns sorted context names derived from embedded compose files.
func contextNames() ([]string, error) {
	files, err := environment.ListEmbeddedFiles()
	if err != nil {
		return nil, err
	}

	var names []string

	for _, file := range files {
		base := filepath.Base(file)
		if base == "compose.yaml" || !strings.HasPrefix(base, "compose.") || !strings.HasSuffix(base, ".yaml") {
			continue
		}

		names = append(names, strings.TrimSuffix(strings.TrimPrefix(base, "compose."), ".yaml"))
	}

	slices.Sort(names)

	return names, nil
}

// Register wires a completion function for a flag the command has defined.
func Register(cmd *cobra.Command, flag string, fn cobra.CompletionFunc) {
	if err := cmd.RegisterFlagCompletionFunc(flag, fn); err != nil {
		cobra.CheckErr(err)
	}
}
