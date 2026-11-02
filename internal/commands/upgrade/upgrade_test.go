package upgrade_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/commands/upgrade"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/xf"
)

const oldPHP = "<?php\npublic static $version = '2.3.0';\npublic static $versionId = 2030071;\n"
const newPHP = "<?php\npublic static $version = '2.3.1';\npublic static $versionId = 2030171;\n"

func TestUpgrade(t *testing.T) {
	if scenario := os.Getenv("XF_UPGRADE_TEST_SCENARIO"); scenario != "" {
		runScenario(t, scenario)
		return
	}

	for _, scenario := range []string{"success", "same", "older", "download", "extract", "metadata", "child-failure"} {
		t.Run(scenario, func(t *testing.T) {
			if scenario == "child-failure" && runtime.GOOS == "windows" {
				t.Skip("tool fixture requires a POSIX shell")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestUpgrade$")
			if dir := os.Getenv("GOCOVERDIR"); dir != "" {
				cmd.Args = append(cmd.Args, "-test.gocoverdir="+dir)
			}
			cmd.Env = append(os.Environ(), "XF_UPGRADE_TEST_SCENARIO="+scenario)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("isolated test: %v\n%s", err, output)
			}
			if scenario == "child-failure" && (!strings.Contains(string(output), "xf up") || !strings.Contains(string(output), "xf xf:upgrade")) {
				t.Fatalf("missing remediation: %s", output)
			}
		})
	}
}

func runScenario(t *testing.T, scenario string) {
	t.Helper()
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	name, content := "upload/src/XF.php", newPHP
	if scenario == "extract" {
		name, content = "upload/../escape", "unsafe"
	}
	w, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-token" || r.URL.Query().Get("license_key") != "license-123" {
			t.Errorf("unexpected request: %s authorization %q", r.URL, r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/customer-oauth2/license-downloadables":
			fmt.Fprint(w, `{"downloadables":[{"download_id":"xenforo","title":"XenForo"},{"download_id":"addon","title":"Add-on"}]}`)
		case "/api/customer-oauth2/license-versions":
			if r.URL.Query().Get("download_id") == "addon" {
				fmt.Fprint(w, `{"versions":[]}`)
			} else {
				fmt.Fprint(w, `{"versions":[{"version_id":2030171,"version_string":"2.3.1","release_date":1700000000}]}`)
			}
		case "/api/customer-oauth2/license-download-info":
			fmt.Fprint(w, `{"download":{"version_string":"2.3.1","filename":"upgrade.zip"}}`)
		case "/api/customer-oauth2/license-download":
			if scenario == "download" {
				http.Error(w, "download unavailable", http.StatusServiceUnavailable)
			} else {
				_, _ = w.Write(archive.Bytes())
			}
		default:
			t.Errorf("unexpected endpoint: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	writeFile(t, configPath, fmt.Sprintf(`{"no_interaction":true,"cache_path":%q,"oauth":{"base_url":%q}}`, filepath.Join(dir, "cache"), server.URL))
	t.Setenv("XF_TOKEN", "test-token")
	t.Setenv("XF_NO_INTERACTION", "true")
	t.Setenv("XF_CACHE_PATH", filepath.Join(dir, "cache"))
	t.Setenv("XF_OAUTH_BASE_URL", server.URL)
	if err := config.Init(configPath); err != nil {
		t.Fatal(err)
	}
	installation := filepath.Join(dir, "installation")
	writeFile(t, filepath.Join(installation, "src/XF.php"), oldPHP)
	writeFile(t, filepath.Join(installation, ".env"), "XF_INSTANCE=my-instance\nCUSTOM=value\n")
	writeFile(t, filepath.Join(installation, "src/config.php"), "user configuration")
	writeFile(t, filepath.Join(installation, "compose.yaml"), "services: {}\n")
	created := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	meta := &xf.Metadata{LicenseKey: "license-123", InstanceName: "my-instance", InstalledProducts: []string{"xenforo", "addon"}, InstalledVersion: "2.3.0", InstalledVersionID: 2030071, CreatedAt: created}
	if err := xf.WriteMetadata(installation, meta); err != nil {
		t.Fatal(err)
	}
	if scenario == "metadata" {
		writeFile(t, filepath.Join(installation, ".xf.json"), "{invalid metadata")
	}
	before := readFile(t, filepath.Join(installation, ".xf.json"))
	target := 2030171
	switch scenario {
	case "same":
		target = 2030071
	case "older":
		target = 2021271
	}
	if scenario == "child-failure" {
		bin := filepath.Join(dir, "bin")
		writeFile(t, filepath.Join(bin, "docker"), "#!/bin/sh\ncase \" $* \" in *' ps '*) exit 0;; *' run '*' php cmd.php xf:upgrade '*) exit 42;; *) exit 99;; esac\n")
		if err := os.Chmod(filepath.Join(bin, "docker"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
	}
	err = upgrade.Run(context.Background(), &upgrade.UpgradeOptions{TargetPath: installation, TargetVersionID: target, SkipUpgrade: scenario != "child-failure"})
	switch scenario {
	case "success", "child-failure":
		if scenario == "success" && err != nil {
			t.Fatal(err)
		}
		if scenario == "child-failure" {
			if code, ok := clierror.ExitCode(err); !ok || code != 42 {
				t.Fatalf("child exit status: %v", err)
			}
		}
		if got := readFile(t, filepath.Join(installation, "src/XF.php")); got != newPHP {
			t.Fatalf("upgraded PHP: %q", got)
		}
		got, err := xf.ReadMetadata(installation)
		if err != nil {
			t.Fatal(err)
		}
		if got.InstalledVersion != "2.3.1" || got.InstalledVersionID != 2030171 || got.LicenseKey != meta.LicenseKey || got.InstanceName != meta.InstanceName || !reflect.DeepEqual(got.InstalledProducts, meta.InstalledProducts) || !got.CreatedAt.Equal(created) {
			t.Fatalf("upgraded metadata: %+v", got)
		}
	default:
		if err == nil {
			t.Fatal("expected upgrade failure")
		}
		if scenario == "same" || scenario == "older" {
			if !errors.Is(err, clierror.ErrInvalidInput) || requests.Load() != 0 {
				t.Fatalf("version rejection: %v, requests %d", err, requests.Load())
			}
		}
		if scenario == "metadata" && !strings.Contains(err.Error(), "metadata") {
			t.Fatalf("malformed metadata: %v", err)
		}
		if scenario == "download" && (!strings.Contains(err.Error(), "download") || requests.Load() == 0) {
			t.Fatalf("download failure: %v, requests %d", err, requests.Load())
		}
		if scenario == "extract" && (!strings.Contains(err.Error(), "extract") || requests.Load() == 0) {
			t.Fatalf("extraction failure: %v, requests %d", err, requests.Load())
		}
		if got := readFile(t, filepath.Join(installation, ".xf.json")); got != before {
			t.Fatalf("metadata changed on failure: %s", got)
		}
		if got := readFile(t, filepath.Join(installation, "src/XF.php")); got != oldPHP {
			t.Fatalf("installed file changed on failure: %q", got)
		}
	}
	if got := readFile(t, filepath.Join(installation, ".env")); got != "XF_INSTANCE=my-instance\nCUSTOM=value\n" {
		t.Fatalf("environment changed: %q", got)
	}
	if got := readFile(t, filepath.Join(installation, "src/config.php")); got != "user configuration" {
		t.Fatalf("user configuration changed: %q", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
