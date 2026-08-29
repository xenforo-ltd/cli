package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var licenseANSI = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripLicenseANSI(s string) string {
	return licenseANSI.ReplaceAllString(s, "")
}

func TestLicenseStatusReflectsValidityAndActivity(t *testing.T) {
	cases := []struct {
		name string
		lic  customerapi.License
		want string
	}{
		{"invalid beats expired", customerapi.License{IsValid: false, IsActive: false}, "Invalid"},
		{"invalid beats active", customerapi.License{IsValid: false, IsActive: true}, "Invalid"},
		{"valid but inactive is expired", customerapi.License{IsValid: true, IsActive: false}, "Expired"},
		{"valid and active", customerapi.License{IsValid: true, IsActive: true}, "Active"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripLicenseANSI(licenseStatus(tc.lic)); got != tc.want {
				t.Errorf("licenseStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLicenseStatusIconMatchesStatus(t *testing.T) {
	cases := []struct {
		name string
		lic  customerapi.License
		want string
	}{
		{"invalid", customerapi.License{IsValid: false}, ui.SymbolError},
		{"expired", customerapi.License{IsValid: true, IsActive: false}, ui.SymbolWarning},
		{"active", customerapi.License{IsValid: true, IsActive: true}, ui.SymbolSuccess},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripLicenseANSI(licenseStatusIcon(tc.lic))
			if got != tc.want {
				t.Errorf("licenseStatusIcon = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormatLicenseSiteFallsBackToADash(t *testing.T) {
	title, url := formatLicenseSite(customerapi.License{})

	if stripLicenseANSI(title) != "—" {
		t.Errorf("missing site title rendered %q, want an em dash", stripLicenseANSI(title))
	}

	if stripLicenseANSI(url) != "—" {
		t.Errorf("missing site URL rendered %q, want an em dash", stripLicenseANSI(url))
	}

	title, url = formatLicenseSite(customerapi.License{SiteTitle: "My Site", SiteURL: "https://example.com"})

	if title != "My Site" {
		t.Errorf("site title = %q, want %q", title, "My Site")
	}

	// The URL must stay unstyled: it sits in a table cell, where an underline
	// would bleed into the column padding.
	if url != "https://example.com" {
		t.Errorf("site URL = %q, want it unstyled", url)
	}

	if strings.Contains(url, "\x1b[") {
		t.Errorf("site URL carries escape codes: %q", url)
	}
}
