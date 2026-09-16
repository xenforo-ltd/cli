package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var errTestBoom = errors.New("boom")

// TestPlannedInitSteps pins the fixed step plan: the Composer and XenForo
// install slots count toward the total whether they run or are printed as
// skipped, so the only thing that changes the total is --skip-up, which makes
// both unreachable.
func TestPlannedInitSteps(t *testing.T) {
	tests := []struct {
		name string
		opts InitOptions
		want int
	}{
		{name: "full run", opts: InitOptions{}, want: 8},
		{name: "skip install", opts: InitOptions{SkipInstall: true}, want: 8},
		{name: "skip composer", opts: InitOptions{SkipComposer: true}, want: 8},
		{name: "skip install and composer", opts: InitOptions{SkipInstall: true, SkipComposer: true}, want: 8},
		{name: "skip up", opts: InitOptions{SkipUp: true}, want: 6},
		{name: "skip up and skip install", opts: InitOptions{SkipUp: true, SkipInstall: true}, want: 6},
		{name: "skip up and skip composer", opts: InitOptions{SkipUp: true, SkipComposer: true}, want: 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plannedInitSteps(tt.opts); got != tt.want {
				t.Fatalf("plannedInitSteps(%+v) = %d, want %d", tt.opts, got, tt.want)
			}
		})
	}
}

func TestParseInstallImportMessage(t *testing.T) {
	if got := parseInstallImportMessage("Importing master data (phrases: 35%)"); got != "importing phrases (35%)" {
		t.Fatalf("unexpected message: %q", got)
	}

	if got := parseInstallImportMessage("some line without marker"); got != "" {
		t.Fatalf("expected empty message, got %q", got)
	}

	if got := parseInstallImportMessage("importing master data ("); got != "importing data" {
		t.Fatalf("expected fallback import message, got %q", got)
	}
}

func TestContainsAnyAndPhaseRules(t *testing.T) {
	if !containsAny("starting containers", []string{"pull", "starting"}) {
		t.Fatal("expected match")
	}

	if containsAny("starting containers", []string{"build", "cached"}) {
		t.Fatal("did not expect match")
	}

	if len(dockerStartPhaseRules()) == 0 || len(installPhaseRules()) == 0 {
		t.Fatal("expected non-empty phase rules")
	}
}

func TestPhaseTrackerWriterProcessLine(t *testing.T) {
	spinner := ui.NewSpinner("base")
	writer := newPhaseTrackerWriter(spinner, "Installing XenForo", installPhaseRules())

	if _, err := writer.Write([]byte("Importing master data (phrases: 20%)\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	tail := writer.TailLines()
	if len(tail) == 0 {
		t.Fatal("expected tail lines")
	}
}

func TestPrepareTargetDirectory(t *testing.T) {
	t.Run("creates missing dir", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "new-dir")
		if err := prepareTargetDirectory(target); err != nil {
			t.Fatalf("prepareTargetDirectory failed: %v", err)
		}

		if info, err := os.Stat(target); err != nil || !info.IsDir() {
			t.Fatalf("target not created as dir: info=%v err=%v", info, err)
		}
	})

	t.Run("rejects file path", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}

		if err := prepareTargetDirectory(file); err == nil {
			t.Fatal("expected error for non-directory target")
		}
	})

	t.Run("non-empty dir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "something.txt"), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}

		if err := prepareTargetDirectory(dir); err == nil {
			t.Fatal("expected error for non-empty non-XenForo directory")
		}
	})

	t.Run("allows non-empty xenforo dir", func(t *testing.T) {
		dir := t.TempDir()

		xfPath := filepath.Join(dir, "src", "XF.php")
		if err := os.MkdirAll(filepath.Dir(xfPath), 0o750); err != nil {
			t.Fatalf("create XF src dir: %v", err)
		}

		if err := os.WriteFile(xfPath, []byte("<?php // XF stub"), 0o600); err != nil {
			t.Fatalf("seed XF.php: %v", err)
		}

		if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed extra file: %v", err)
		}

		if err := prepareTargetDirectory(dir); err != nil {
			t.Fatalf("prepareTargetDirectory should allow non-empty XenForo directory: %v", err)
		}
	})
}

func TestHelpersFormatting(t *testing.T) {
	if got := formatProductNames([]string{"xenforo", "xfmg"}, map[string]string{"xenforo": "XenForo", "xfmg": "Media Gallery"}); got != "XenForo, Media Gallery" {
		t.Fatalf("unexpected product names: %q", got)
	}

	if got := splitCSV("a, b,,c"); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("unexpected splitCSV: %#v", got)
	}

	lic := customerapi.License{LicenseKey: "ABC", SiteTitle: "Site", SiteURL: "https://example.com"}
	if got := licenseOptionLabel(lic); !strings.Contains(got, "ABC") || !strings.Contains(got, "Site") {
		t.Fatalf("unexpected license label: %q", got)
	}

	if got := formatProductList([]string{"xenforo", "xfmg"}, map[string]string{"xenforo": "XenForo", "xfmg": "Media"}); got != "XenForo, Media" {
		t.Fatalf("unexpected product list: %q", got)
	}
}

func TestInferSiteTitleAndBoardFallback(t *testing.T) {
	opts := &InitOptions{InstanceName: "demo", EnvResolved: map[string]string{"XF_TITLE": "Forum [demo]"}}
	if got := inferSiteTitleFromEnv(opts); got != "Forum" {
		t.Fatalf("unexpected inferred title: %q", got)
	}

	fallback := fallbackBoardURL("demo")
	if !strings.Contains(fallback, "demo") {
		t.Fatalf("unexpected fallback board URL: %q", fallback)
	}

	url, detected := chooseBoardURL("demo", "", errTestBoom)
	if detected || url != fallback {
		t.Fatalf("unexpected chooseBoardURL fallback: url=%q detected=%v", url, detected)
	}
}

func TestNormalizeRuntimeStoragePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions")
	}

	target := t.TempDir()

	nestedDir := filepath.Join(target, "data", "nested")
	if err := os.MkdirAll(nestedDir, 0o700); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	nestedFile := filepath.Join(nestedDir, "cache.txt")
	if err := os.WriteFile(nestedFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed nested file: %v", err)
	}

	if err := normalizeRuntimeStorage(target); err != nil {
		t.Fatalf("normalizeRuntimeStorage failed: %v", err)
	}

	// The seeded data/ tree and the newly created internal_data/ must all be
	// traversable/writable by the container's www-data user.
	for _, dir := range []string{
		filepath.Join(target, "data"),
		nestedDir,
		filepath.Join(target, "internal_data"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}

		if got := info.Mode().Perm(); got != 0o777 {
			t.Fatalf("%s mode = %o, want 0777", dir, got)
		}
	}

	info, err := os.Stat(nestedFile)
	if err != nil {
		t.Fatalf("stat nested file: %v", err)
	}

	if got := info.Mode().Perm(); got != 0o666 {
		t.Fatalf("nested file mode = %o, want 0666", got)
	}
}
