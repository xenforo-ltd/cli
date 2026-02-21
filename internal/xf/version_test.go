package xf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseVersionStringAndIDRoundTrip(t *testing.T) {
	v, err := ParseVersionString("2.3.8")
	if err != nil {
		t.Fatalf("ParseVersionString failed: %v", err)
	}
	if v.ID != 2030871 {
		t.Fatalf("ID = %d, want 2030871", v.ID)
	}

	parsed := ParseVersionID(v.ID)
	if parsed.Major != 2 || parsed.Minor != 3 || parsed.Patch != 8 {
		t.Fatalf("unexpected parsed version: %#v", parsed)
	}
}

func TestParseVersionStringStabilityVariants(t *testing.T) {
	cases := []struct {
		in        string
		stability string
	}{
		{"2.3.8 Alpha 2", "alpha"},
		{"2.3.8 Beta 1", "beta"},
		{"2.3.8 RC 3", "rc"},
		{"2.3.8 PL 2", "pl"},
	}

	for _, tc := range cases {
		v, err := ParseVersionString(tc.in)
		if err != nil {
			t.Fatalf("ParseVersionString(%q) failed: %v", tc.in, err)
		}
		if v.Stability != tc.stability {
			t.Fatalf("stability = %q, want %q", v.Stability, tc.stability)
		}
	}
}

func TestVersionComparisons(t *testing.T) {
	a := ParseVersionID(2030871)
	b := ParseVersionID(2030971)
	c := ParseVersionID(2030871)

	if !b.IsNewerThan(a) {
		t.Fatal("expected b newer than a")
	}
	if !a.IsOlderThan(b) {
		t.Fatal("expected a older than b")
	}
	if a.Compare(c) != 0 {
		t.Fatal("expected equal comparison")
	}
}

func TestDetectVersion(t *testing.T) {
	dir := t.TempDir()
	xfPath := filepath.Join(dir, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `<?php
class XF {
    public static $version = '2.3.8';
    public static $versionId = 2030871;
}`
	if err := os.WriteFile(xfPath, []byte(content), 0644); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	v, err := DetectVersion(dir)
	if err != nil {
		t.Fatalf("DetectVersion failed: %v", err)
	}
	if v.ID != 2030871 || v.String != "2.3.8" {
		t.Fatalf("unexpected version: %#v", v)
	}
}

func TestDetectVersionMissingFile(t *testing.T) {
	if _, err := DetectVersion(t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}
