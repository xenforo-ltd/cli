package doctor

import (
	"fmt"
	"github.com/xenforo-ltd/cli/internal/auth"
	"testing"

	"github.com/xenforo-ltd/cli/internal/ui"
)

func TestCheckStatusStringAndSymbol(t *testing.T) {
	cases := []struct {
		status CheckStatus
		str    string
		sym    string
	}{
		{StatusOK, "OK", "+"},
		{StatusWarning, "WARNING", "!"},
		{StatusError, "ERROR", "x"},
		{StatusSkipped, "SKIPPED", "-"},
		{CheckStatus(999), "UNKNOWN", "?"},
	}

	for _, tc := range cases {
		if got := tc.status.String(); got != tc.str {
			t.Fatalf("String() = %q, want %q", got, tc.str)
		}

		if got := tc.status.Symbol(); got != tc.sym {
			t.Fatalf("Symbol() = %q, want %q", got, tc.sym)
		}
	}
}

func TestDoctorHasErrorsAndWarnings(t *testing.T) {
	d := &Doctor{results: []*CheckResult{{Status: StatusOK}}}
	if d.HasErrors() || d.HasWarnings() {
		t.Fatal("expected no errors or warnings")
	}

	d.results = append(d.results, &CheckResult{Status: StatusWarning})
	if d.HasErrors() {
		t.Fatal("expected no errors")
	}

	if !d.HasWarnings() {
		t.Fatal("expected warning")
	}

	d.results = append(d.results, &CheckResult{Status: StatusError})
	if !d.HasErrors() {
		t.Fatal("expected error")
	}
}

func TestFormatBytes(t *testing.T) {
	if got := ui.FormatBytes(512); got != "512 B" {
		t.Fatalf("FormatBytes(512) = %q", got)
	}

	if got := ui.FormatBytes(1024); got != "1.0 KB" {
		t.Fatalf("FormatBytes(1024) = %q", got)
	}

	if got := ui.FormatBytes(2 * 1024 * 1024); got != "2.0 MB" {
		t.Fatalf("FormatBytes(2MB) = %q", got)
	}
}

func TestEnvironmentAuthenticationSkipsKeychain(t *testing.T) {
	d := NewDoctor()
	d.checkAuthentication("environment", &auth.Token{External: true}, nil)
	if len(d.results) != 2 || d.results[0].Status != StatusOK {
		t.Fatal("environment credentials require keychain")
	}
	result := d.results[1]
	if result.Status != StatusOK || result.Message != "XF_TOKEN is present (validity and expiry not checked)" {
		t.Fatalf("unexpected auth diagnosis: %v", result)
	}
}

func TestCredentialFailuresAreNotReportedAsLoggedOut(t *testing.T) {
	for _, err := range []error{auth.ErrStoreUnavailable, fmt.Errorf("insecure credential file: %w", auth.ErrInvalidInput)} {
		d := NewDoctor()
		d.checkAuthentication("file", nil, err)
		if d.results[0].Name != "Credential Storage" || d.results[0].Status != StatusError || d.results[1].Status != StatusSkipped {
			t.Fatalf("misleading diagnosis: %+v / %+v", d.results[0], d.results[1])
		}
	}
	d := NewDoctor()
	d.checkAuthentication("file", nil, auth.ErrAuthRequired)
	if d.HasErrors() || d.results[1].Message != "Not authenticated" {
		t.Fatal("missing credentials reported as broken storage")
	}
}
