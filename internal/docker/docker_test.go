package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractDockerFilesWithOptions_NoOverwriteBaseFiles(t *testing.T) {
	tmp := t.TempDir()

	custom := []byte("services:\n  custom: {}\n")

	composePath := filepath.Join(tmp, "compose.yaml")
	if err := os.WriteFile(composePath, custom, 0o644); err != nil {
		t.Fatalf("write custom compose: %v", err)
	}

	if err := ExtractDockerFilesWithOptions(tmp, ExtractOptions{OverwriteBaseFiles: false}); err != nil {
		t.Fatalf("extract docker files: %v", err)
	}

	got, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}

	if string(got) != string(custom) {
		t.Fatalf("compose.yaml was overwritten unexpectedly")
	}

	if _, err := os.Stat(filepath.Join(tmp, "Dockerfile")); err != nil {
		t.Fatalf("expected Dockerfile to be written: %v", err)
	}
}

func TestExtractDockerFilesWithOptions_OverwriteBaseFiles(t *testing.T) {
	tmp := t.TempDir()

	composePath := filepath.Join(tmp, "compose.yaml")
	if err := os.WriteFile(composePath, []byte("services:\n  custom: {}\n"), 0o644); err != nil {
		t.Fatalf("write custom compose: %v", err)
	}

	if err := ExtractDockerFilesWithOptions(tmp, ExtractOptions{OverwriteBaseFiles: true}); err != nil {
		t.Fatalf("extract docker files: %v", err)
	}

	want, err := GetDockerFile("compose.yaml")
	if err != nil {
		t.Fatalf("read embedded compose: %v", err)
	}

	got, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("compose.yaml not overwritten with embedded content")
	}
}

func TestExtractDockerFilesWithOptions_DefaultFileBehaviorUnchanged(t *testing.T) {
	tmp := t.TempDir()
	envPath := filepath.Join(tmp, ".env")

	customEnv := []byte("XF_INSTANCE=custom\n")
	if err := os.WriteFile(envPath, customEnv, 0o644); err != nil {
		t.Fatalf("write custom env: %v", err)
	}

	if err := ExtractDockerFilesWithOptions(tmp, ExtractOptions{OverwriteBaseFiles: false}); err != nil {
		t.Fatalf("extract docker files: %v", err)
	}

	gotEnv, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}

	if string(gotEnv) != string(customEnv) {
		t.Fatalf(".env should remain unchanged")
	}

	defaultPath := filepath.Join(tmp, ".env.default")
	if _, err := os.Stat(defaultPath); err != nil {
		t.Fatalf("expected .env.default to be generated: %v", err)
	}
}
