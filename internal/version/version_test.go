package version

import (
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	info := Get()

	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", info.GoVersion, runtime.Version())
	}
	if info.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", info.OS, runtime.GOOS)
	}
	if info.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", info.Arch, runtime.GOARCH)
	}

	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(info.Version) {
		t.Errorf("Version = %q, want format v<major>.<minor>.<patch>", info.Version)
	}
	if info.Commit != "unknown" {
		t.Errorf("Commit = %q, want %q", info.Commit, "unknown")
	}
	if info.Date != "unknown" {
		t.Errorf("Date = %q, want %q", info.Date, "unknown")
	}
}

func TestInfo_String(t *testing.T) {
	info := Info{
		Version:   "1.0.0",
		Commit:    "abc1234",
		Date:      "2026-01-26",
		GoVersion: "go1.22.4",
		OS:        "darwin",
		Arch:      "arm64",
	}

	str := info.String()

	expectedParts := []string{
		"xf",
		"1.0.0",
		"abc1234",
		"2026-01-26",
		"go1.22.4",
		"darwin/arm64",
	}

	for _, part := range expectedParts {
		if !strings.Contains(str, part) {
			t.Errorf("String() missing %q, got: %s", part, str)
		}
	}
}

func TestInfo_Short(t *testing.T) {
	info := Info{
		Version: "2.3.4",
	}

	if short := info.Short(); short != "2.3.4" {
		t.Errorf("Short() = %q, want %q", short, "2.3.4")
	}
}
