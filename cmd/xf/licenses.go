package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var licensesCmd = &cobra.Command{
	Use:   "licenses",
	Short: "List your XenForo licenses",
	Long: `Display all XenForo licenses associated with your customer account.

Shows license details including product, status, expiration date, site URL,
and available extras (add-ons).`,
	Example: `  # List all licenses (compact table)
  xf licenses

  # List with full details
  xf licenses -v

  # Output as JSON (useful for scripting)
  xf licenses --json`,
	Args:    cobra.NoArgs,
	GroupID: "start",
	RunE:    runLicenses,
}

var flagLicensesJSON bool

func init() {
	licensesCmd.Flags().BoolVar(&flagLicensesJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(licensesCmd)
}

func runLicenses(cmd *cobra.Command, args []string) error {
	client, err := customerapi.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create customer API client: %w", err)
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	licenses, err := client.GetLicenses(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch licenses: %w", err)
	}

	if flagLicensesJSON {
		type jsonExtra struct {
			ExtraID        string `json:"extra_id"`
			Name           string `json:"name"`
			IsDownloadable bool   `json:"is_downloadable"`
		}

		type jsonLicense struct {
			LicenseKey     string      `json:"license_key"`
			ProductID      string      `json:"product_id"`
			ProductTitle   string      `json:"product_title"`
			IsValid        bool        `json:"is_valid"`
			IsActive       bool        `json:"is_active"`
			StartDate      string      `json:"start_date,omitempty"`
			ExpirationDate string      `json:"expiration_date,omitempty"`
			SiteURL        string      `json:"site_url,omitempty"`
			SiteTitle      string      `json:"site_title,omitempty"`
			CanDownload    bool        `json:"can_download"`
			Extras         []jsonExtra `json:"extras,omitempty"`
		}

		output := make([]jsonLicense, 0, len(licenses))
		for _, lic := range licenses {
			jl := jsonLicense{
				LicenseKey:   lic.LicenseKey,
				ProductID:    lic.ProductID,
				ProductTitle: lic.ProductTitle,
				IsValid:      lic.IsValid,
				IsActive:     lic.IsActive,
				SiteURL:      lic.SiteURL,
				SiteTitle:    lic.SiteTitle,
				CanDownload:  lic.CanDownload,
			}

			if !lic.StartDate.IsZero() {
				jl.StartDate = lic.StartDate.Format(time.RFC3339)
			}

			if !lic.ExpirationDate.IsZero() {
				jl.ExpirationDate = lic.ExpirationDate.Format(time.RFC3339)
			}

			if len(lic.Extras) > 0 {
				jl.Extras = make([]jsonExtra, 0, len(lic.Extras))
				for _, extra := range lic.Extras {
					jl.Extras = append(jl.Extras, jsonExtra{
						ExtraID:        extra.ExtraID,
						Name:           extra.Name,
						IsDownloadable: extra.IsDownloadable,
					})
				}
			}

			output = append(output, jl)
		}

		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal licenses: %w", err)
		}

		ui.Println(string(data))

		return nil
	}

	if len(licenses) == 0 {
		ui.PrintEmpty("No licenses found")
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.Verbose {
		runLicensesVerbose(licenses)
		return nil
	}

	runLicensesTable(licenses)

	return nil
}

// licenseStatus renders the colored status label for a license.
func licenseStatus(lic customerapi.License) string {
	switch {
	case !lic.IsValid:
		return ui.Error.Render("Invalid")
	case !lic.IsActive:
		return ui.Warning.Render("Expired")
	default:
		return ui.Success.Render("Active")
	}
}

// licenseStatusIcon renders the colored status icon for a license.
func licenseStatusIcon(lic customerapi.License) string {
	switch {
	case !lic.IsValid:
		return ui.StatusIcon("error")
	case !lic.IsActive:
		return ui.StatusIcon("warning")
	default:
		return ui.StatusIcon("success")
	}
}

func runLicensesTable(licenses []customerapi.License) {
	ui.PrintInfo("Found " + ui.Plural(len(licenses), "license", "licenses"))
	ui.Println()

	headers := []string{"LICENSE", "SITE TITLE", "SITE URL", "PRODUCT", "STATUS", "EXPIRES", "DOWNLOAD"}
	rows := make([][]string, 0, len(licenses))

	for _, lic := range licenses {
		siteTitle, siteURL := formatLicenseSite(lic)

		status := licenseStatus(lic)

		var expires string

		if !lic.ExpirationDate.IsZero() {
			if lic.ExpirationDate.After(time.Now()) {
				expires = ui.FormatDate(lic.ExpirationDate.Time)
			} else {
				expires = ui.Warning.Render(ui.FormatDate(lic.ExpirationDate.Time))
			}
		} else {
			expires = ui.Success.Render("Lifetime")
		}

		var download string
		if lic.CanDownload {
			download = ui.Success.Render("Yes")
		} else {
			download = ui.Dim.Render("No")
		}

		rows = append(rows, []string{
			lic.LicenseKey,
			siteTitle,
			siteURL,
			lic.ProductTitle,
			status,
			expires,
			download,
		})
	}

	ui.Println(ui.NewTable(headers, rows))
	ui.Println()
	ui.PrintHint("Run " + ui.Command.Render("xf licenses -v") + " for detailed license information")
}

func formatLicenseSite(lic customerapi.License) (string, string) {
	siteTitle := ui.Dim.Render("—")
	if lic.SiteTitle != "" {
		siteTitle = lic.SiteTitle
	}

	siteURL := ui.Dim.Render("—")
	if lic.SiteURL != "" {
		siteURL = lic.SiteURL
	}

	return siteTitle, siteURL
}

func runLicensesVerbose(licenses []customerapi.License) {
	ui.PrintInfo("Found " + ui.Plural(len(licenses), "license", "licenses"))
	ui.Println()

	for i, lic := range licenses {
		ui.Printf("%s %s %s\n", licenseStatusIcon(lic), ui.Bold.Render(lic.ProductTitle), ui.Muted.Render(lic.LicenseKey))

		siteTitle, siteURL := formatLicenseSite(lic)
		pairs := []ui.KVPair{
			ui.KV("License key", lic.LicenseKey),
			ui.KV("Status", licenseStatus(lic)),
			ui.KV("Site title", siteTitle),
			ui.KV("Site URL", siteURL),
		}

		if !lic.StartDate.IsZero() {
			pairs = append(pairs, ui.KV("Purchased", ui.FormatDate(lic.StartDate.Time)))
		}

		if !lic.ExpirationDate.IsZero() {
			if lic.ExpirationDate.After(time.Now()) {
				pairs = append(pairs, ui.KV("Expires", ui.FormatDate(lic.ExpirationDate.Time)))
			} else {
				pairs = append(pairs, ui.KV("Expires", ui.Warning.Render(ui.FormatDate(lic.ExpirationDate.Time)+" (expired)")))
			}
		} else {
			pairs = append(pairs, ui.KV("Expires", ui.Success.Render("Lifetime")))
		}

		if lic.CanDownload {
			pairs = append(pairs, ui.KV("Download", ui.Success.Render("Yes")))
		} else {
			pairs = append(pairs, ui.KV("Download", ui.Dim.Render("No")))
		}

		ui.PrintKeyValuePadded(pairs)

		if len(lic.Extras) > 0 {
			ui.Println(ui.Indent1 + ui.Bold.Render("Extras"))

			for _, extra := range lic.Extras {
				note := ""
				if extra.IsDownloadable {
					note = ui.Dim.Render(" (downloadable)")
				}

				ui.Printf("%s%s %s%s\n", ui.Indent1, ui.Dim.Render(ui.SymbolBullet), extra.Name, note)
			}
		}

		if i < len(licenses)-1 {
			ui.Println()
		}
	}
}
