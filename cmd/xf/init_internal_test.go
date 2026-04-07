package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/ui"
)

var errTestBoom = errors.New("boom")

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
