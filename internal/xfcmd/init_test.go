package xfcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRejectsNonXenForoDirectory(t *testing.T) {
	err := Init(t.TempDir(), InitOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInitCreatesDockerFilesForXenForoDirectory(t *testing.T) {
	dir := t.TempDir()

	xfPath := filepath.Join(dir, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfPath), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(xfPath, []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	err := Init(dir, InitOptions{Contexts: []string{"caddy", "mysql"}})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	mustExist := []string{
		"compose.yaml",
		"Dockerfile",
		".env",
		".dockerignore",
	}
	for _, name := range mustExist {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}

	envData, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}

	if !strings.Contains(string(envData), "XF_CONTEXTS=caddy:mysql") {
		t.Fatalf("expected overridden contexts in .env, got:\n%s", string(envData))
	}
}

func TestInitDoesNotOverwriteXenForoCoreFiles(t *testing.T) {
	dir := t.TempDir()

	xfPath := filepath.Join(dir, "src", "XF.php")
	if err := os.MkdirAll(filepath.Dir(xfPath), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	originalXF := "<?php\n// xenforo core marker\n"
	if err := os.WriteFile(xfPath, []byte(originalXF), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	existingCore := filepath.Join(dir, "src", "Entity", "User.php")
	if err := os.MkdirAll(filepath.Dir(existingCore), 0o750); err != nil {
		t.Fatalf("mkdir core dir: %v", err)
	}

	coreContent := "<?php\n// custom core file\n"
	if err := os.WriteFile(existingCore, []byte(coreContent), 0o600); err != nil {
		t.Fatalf("write core file: %v", err)
	}

	if err := InitExisting(dir, InitOptions{OverwriteExisting: true}); err != nil {
		t.Fatalf("InitExisting failed: %v", err)
	}

	gotXF, err := os.ReadFile(xfPath)
	if err != nil {
		t.Fatalf("read XF.php: %v", err)
	}

	if string(gotXF) != originalXF {
		t.Fatalf("XF.php was modified unexpectedly")
	}

	gotCore, err := os.ReadFile(existingCore)
	if err != nil {
		t.Fatalf("read core file: %v", err)
	}

	if string(gotCore) != coreContent {
		t.Fatalf("existing XenForo core file was modified unexpectedly")
	}
}
