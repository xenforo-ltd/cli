package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xenforo-ltd/cli/internal/xf"
)

func TestChooseBoardURL(t *testing.T) {
	url, detected := chooseBoardURL("demo", "http://localhost:8080", nil)
	if !detected {
		t.Fatal("expected detected URL to be used")
	}
	if url != "http://localhost:8080" {
		t.Fatalf("url = %q", url)
	}

	url, detected = chooseBoardURL("demo", "", nil)
	if detected {
		t.Fatal("expected fallback for empty detected URL")
	}
	if url != fallbackBoardURL("demo") {
		t.Fatalf("url = %q, want fallback", url)
	}

	url, detected = chooseBoardURL("demo", "http://localhost:8080", errors.New("failed"))
	if detected {
		t.Fatal("expected fallback when detection errors")
	}
	if url != fallbackBoardURL("demo") {
		t.Fatalf("url = %q, want fallback", url)
	}
}

func TestConfigureEnvironmentSetsContextsWhenProvided(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	initial := "XF_CONTEXTS=legacy\nXF_INSTANCE=old\n"
	if err := os.WriteFile(envPath, []byte(initial), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	opts := &InitOptions{
		TargetPath:   dir,
		InstanceName: "demo",
		AdminEmail:   "admin@example.com",
		SiteTitle:    "XenForo",
		Contexts:     []string{"caddy", "mysql"},
	}
	if err := configureEnvironment(opts); err != nil {
		t.Fatalf("configureEnvironment: %v", err)
	}

	env, err := xf.ReadEnvFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if env["XF_CONTEXTS"] != "caddy:mysql" {
		t.Fatalf("XF_CONTEXTS = %q, want %q", env["XF_CONTEXTS"], "caddy:mysql")
	}
}

func TestConfigureEnvironmentPreservesContextsWhenNotProvided(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	initial := "XF_CONTEXTS=legacy\nXF_INSTANCE=old\n"
	if err := os.WriteFile(envPath, []byte(initial), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	opts := &InitOptions{
		TargetPath:   dir,
		InstanceName: "demo",
		AdminEmail:   "admin@example.com",
		SiteTitle:    "XenForo",
	}
	if err := configureEnvironment(opts); err != nil {
		t.Fatalf("configureEnvironment: %v", err)
	}

	env, err := xf.ReadEnvFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if env["XF_CONTEXTS"] != "legacy" {
		t.Fatalf("XF_CONTEXTS = %q, want legacy", env["XF_CONTEXTS"])
	}
}

func TestConfigureEnvironmentAppliesEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	initial := "XF_CONTEXTS=legacy\nXF_INSTANCE=old\n"
	if err := os.WriteFile(envPath, []byte(initial), 0644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	opts := &InitOptions{
		TargetPath:   dir,
		InstanceName: "demo",
		AdminEmail:   "admin@example.com",
		SiteTitle:    "XenForo",
		EnvResolved: map[string]string{
			"XF_DEBUG": "0",
		},
	}
	if err := configureEnvironment(opts); err != nil {
		t.Fatalf("configureEnvironment: %v", err)
	}

	env, err := xf.ReadEnvFile(envPath)
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if env["XF_DEBUG"] != "0" {
		t.Fatalf("XF_DEBUG = %q, want 0", env["XF_DEBUG"])
	}
}

func TestEnsureCoreFirstUnique(t *testing.T) {
	got := ensureCoreFirstUnique([]string{"xfmg", "xenforo", "xfmg", "xfes"})
	want := []string{"xenforo", "xfmg", "xfes"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEffectiveContextsDefaultsAndNormalization(t *testing.T) {
	defaulted := effectiveContexts(&InitOptions{})
	if len(defaulted) == 0 {
		t.Fatal("expected default contexts")
	}
	expected := "caddy,mysql,development,caddy-development,redis,mailpit"
	if strings.Join(defaulted, ",") != expected {
		t.Fatalf("default contexts = %q, want %q", strings.Join(defaulted, ","), expected)
	}

	normalized := effectiveContexts(&InitOptions{Contexts: []string{"caddy", "mysql", "caddy", "  "}})
	if strings.Join(normalized, ",") != "caddy,caddy-development,mysql" {
		t.Fatalf("normalized contexts = %q", strings.Join(normalized, ","))
	}
}

func TestCurrentEnvPreviewHidesDefaultDebugValues(t *testing.T) {
	merged, _ := currentEnvPreview(&InitOptions{
		InstanceName: "demo",
		AdminEmail:   "admin@example.com",
		SiteTitle:    "XenForo",
		EnvResolved: map[string]string{
			"XF_DEBUG":       "1",
			"XF_DEVELOPMENT": "1",
		},
		EnvSources: map[string]string{
			"XF_DEBUG":       "override",
			"XF_DEVELOPMENT": "override",
		},
	})

	if _, ok := merged["XF_DEBUG"]; ok {
		t.Fatal("expected XF_DEBUG to be hidden when set to default")
	}
	if _, ok := merged["XF_DEVELOPMENT"]; ok {
		t.Fatal("expected XF_DEVELOPMENT to be hidden when set to default")
	}
}

func TestCurrentEnvPreviewShowsNonDefaultDebugValues(t *testing.T) {
	merged, sources := currentEnvPreview(&InitOptions{
		InstanceName: "demo",
		AdminEmail:   "admin@example.com",
		SiteTitle:    "XenForo",
		EnvResolved: map[string]string{
			"XF_DEBUG":       "0",
			"XF_DEVELOPMENT": "0",
		},
		EnvSources: map[string]string{
			"XF_DEBUG":       "override",
			"XF_DEVELOPMENT": "override",
		},
	})

	if merged["XF_DEBUG"] != "0" || sources["XF_DEBUG"] != "override" {
		t.Fatalf("unexpected XF_DEBUG preview: value=%q source=%q", merged["XF_DEBUG"], sources["XF_DEBUG"])
	}
	if merged["XF_DEVELOPMENT"] != "0" || sources["XF_DEVELOPMENT"] != "override" {
		t.Fatalf("unexpected XF_DEVELOPMENT preview: value=%q source=%q", merged["XF_DEVELOPMENT"], sources["XF_DEVELOPMENT"])
	}
}

func TestValidateReviewInputs(t *testing.T) {
	valid := &InitOptions{
		AdminUser:     "admin",
		AdminPassword: "secret",
		AdminEmail:    "admin@example.com",
		EnvResolved: map[string]string{
			"XF_TITLE": "XenForo",
		},
	}
	if err := validateReviewInputs(valid); err != nil {
		t.Fatalf("expected valid inputs, got: %v", err)
	}

	invalidEmail := *valid
	invalidEmail.AdminEmail = "no-at"
	if err := validateReviewInputs(&invalidEmail); err == nil {
		t.Fatal("expected invalid email error")
	}

	invalidEnv := *valid
	invalidEnv.EnvResolved = map[string]string{"BAD KEY": "x"}
	if err := validateReviewInputs(&invalidEnv); err == nil {
		t.Fatal("expected invalid env key error")
	}

	newlineEnv := *valid
	newlineEnv.EnvResolved = map[string]string{"XF_TITLE": "line1\nline2"}
	if err := validateReviewInputs(&newlineEnv); err == nil {
		t.Fatal("expected newline env value error")
	}
}
