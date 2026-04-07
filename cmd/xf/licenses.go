package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
and available extras (add-ons).

Examples:
  # List all licenses (compact table)
  xf licenses

  # List with full details
  xf licenses -v

  # Output as JSON (useful for scripting)
  xf licenses --json`,
	RunE: runLicenses,
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
		ui.PrintInfo("No licenses found.")
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

func runLicensesTable(licenses []customerapi.License) {
	ui.Printf("%s Found %s license(s)\n\n", ui.StatusIcon("success"), ui.Bold.Render(strconv.Itoa(len(licenses))))

	headers := []string{"LICENSE", "SITE TITLE", "SITE URL", "PRODUCT", "STATUS", "EXPIRES", "DOWNLOAD"}
	rows := make([][]string, 0, len(licenses))

	for _, lic := range licenses {
		siteTitle, siteURL := formatLicenseSite(lic)

		var status string

		switch {
		case !lic.IsValid:
			status = ui.Error.Render("Invalid")
		case !lic.IsActive:
			status = ui.Warning.Render("Expired")
		default:
			status = ui.Success.Render("Active")
		}

		var expires string

		if !lic.ExpirationDate.IsZero() {
			if lic.ExpirationDate.After(time.Now()) {
				expires = lic.ExpirationDate.Format("2006-01-02")
			} else {
				expires = ui.Warning.Render(lic.ExpirationDate.Format("2006-01-02"))
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
	ui.Printf("\nUse %s for detailed license information.\n", ui.Command.Render("-v"))
}

func formatLicenseSite(lic customerapi.License) (string, string) {
	siteTitle := "N/A"
	if lic.SiteTitle != "" {
		siteTitle = lic.SiteTitle
	}

	siteURL := "N/A"
	if lic.SiteURL != "" {
		siteURL = ui.URL.Render(lic.SiteURL)
	}

	return siteTitle, siteURL
}

func runLicensesVerbose(licenses []customerapi.License) {
	ui.Printf("%s Found %s license(s)\n\n", ui.StatusIcon("success"), ui.Bold.Render(strconv.Itoa(len(licenses))))

	for i, lic := range licenses {
		var statusText string

		switch {
		case !lic.IsValid:
			statusText = ui.Error.Render("Invalid")
		case !lic.IsActive:
			statusText = ui.Warning.Render("Expired")
		default:
			statusText = ui.Success.Render("Active")
		}

		ui.Printf("%s %s\n", ui.StatusIcon("success"), ui.Bold.Render(lic.ProductTitle))

		siteTitle, siteURL := formatLicenseSite(lic)
		pairs := []ui.KVPair{
			ui.KV("License Key", lic.LicenseKey),
			ui.KV("Status", statusText),
			ui.KV("Site Title", siteTitle),
			ui.KV("Site URL", siteURL),
		}

		if !lic.StartDate.IsZero() {
			pairs = append(pairs, ui.KV("Purchased", lic.StartDate.Format("2006-01-02")))
		}

		if !lic.ExpirationDate.IsZero() {
			if lic.ExpirationDate.After(time.Now()) {
				pairs = append(pairs, ui.KV("Expires", lic.ExpirationDate.Format("2006-01-02")))
			} else {
				pairs = append(pairs, ui.KV("Expired", ui.Warning.Render(lic.ExpirationDate.Format("2006-01-02"))))
			}
		} else {
			pairs = append(pairs, ui.KV("Expires", ui.Success.Render("Never (Lifetime)")))
		}

		if lic.CanDownload {
			pairs = append(pairs, ui.KV("Download", ui.Success.Render("Available")))
		} else {
			pairs = append(pairs, ui.KV("Download", ui.Dim.Render("Not available")))
		}

		ui.PrintKeyValuePadded(pairs)

		if len(lic.Extras) > 0 {
			ui.Printf("\n%sExtras:\n", ui.Indent1)

			for _, extra := range lic.Extras {
				downloadable := ""
				if extra.IsDownloadable {
					downloadable = ui.Success.Render(" (downloadable)")
				}

				ui.Printf("%s%s %s%s\n", ui.Indent2, ui.Dim.Render(ui.SymbolBullet), extra.Name, downloadable)
			}
		}

		if i < len(licenses)-1 {
			ui.Println()
		}
	}
}
