package doctor

import "testing"

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

func TestTextHelpers(t *testing.T) {
	lines := splitLines("a\nb\n")
	if len(lines) != 2 || lines[0] != "a" || lines[1] != "b" {
		t.Fatalf("splitLines unexpected: %#v", lines)
	}

	fields := splitFields("  one\t two   three ")
	if len(fields) != 3 || fields[0] != "one" || fields[1] != "two" || fields[2] != "three" {
		t.Fatalf("splitFields unexpected: %#v", fields)
	}

	joined := joinStrings([]string{"a", "b", "c"}, "|")
	if joined != "a|b|c" {
		t.Fatalf("joinStrings = %q", joined)
	}
	if joinStrings(nil, ",") != "" {
		t.Fatal("joinStrings(nil) should be empty")
	}
}

func TestFormatBytes(t *testing.T) {
	if got := formatBytes(512); got != "512 B" {
		t.Fatalf("formatBytes(512) = %q", got)
	}
	if got := formatBytes(1024); got != "1.0 KB" {
		t.Fatalf("formatBytes(1024) = %q", got)
	}
	if got := formatBytes(2 * 1024 * 1024); got != "2.0 MB" {
		t.Fatalf("formatBytes(2MB) = %q", got)
	}
}
