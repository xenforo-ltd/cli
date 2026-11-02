package initialize_test

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
	"sync"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/commands/initialize"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/xf"
)

func TestInitialize(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell and Unix file permissions")
	}
	if os.Getenv("XF_INITIALIZE_TEST_CHILD") != "1" {
		// Configuration is cached for the process lifetime.
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInitialize$")
		if dir := os.Getenv("GOCOVERDIR"); dir != "" {
			cmd.Args = append(cmd.Args, "-test.gocoverdir="+dir)
		}
		cmd.Env = append(os.Environ(), "XF_INITIALIZE_TEST_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated initialize test: %v\n%s", err, out)
		}
		return
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, content := range map[string]string{"upload/src/XF.php": "core", "upload/data/nested/file": "data", "upload/internal_data/file": "internal", "upload/addon.txt": "addon"} {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	downloads := map[string]int{}
	failVersions := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-token" || r.URL.Query().Get("license_key") != "license" {
			t.Errorf("unexpected request: %s %v", r.URL, r.Header)
		}
		switch r.URL.Path {
		case "/api/customer-oauth2/license-downloadables":
			fmt.Fprint(w, `{"downloadables":[{"download_id":"addon","title":"Add-on"}]}`)
		case "/api/customer-oauth2/license-versions":
			mu.Lock()
			fail := failVersions
			mu.Unlock()
			if fail {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprint(w, `{"versions":[{"version_id":10,"version_string":"2.3.0","release_date":1700000000}]}`)
		case "/api/customer-oauth2/license-download-info":
			fmt.Fprint(w, `{"download":{"version_string":"2.3.0","filename":"package.zip"}}`)
		case "/api/customer-oauth2/license-download":
			mu.Lock()
			downloads[r.URL.Query().Get("download_id")]++
			mu.Unlock()
			_, _ = w.Write(archive.Bytes())
		default:
			t.Errorf("unexpected endpoint %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\ncase \"$*\" in 'info'|'compose version') exit 0 ;; *) exit 1 ;; esac\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("XF_TOKEN", "local-token")
	t.Setenv("XF_NO_INTERACTION", "true")
	t.Setenv("XF_OAUTH_BASE_URL", server.URL)
	t.Setenv("XF_CACHE_PATH", filepath.Join(dir, "cache"))
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.Init(configPath); err != nil {
		t.Fatal(err)
	}
	valid := func(target string) *initialize.InitOptions {
		return &initialize.InitOptions{TargetPath: target, LicenseKey: "license", VersionID: 10, AdminUser: "admin", AdminPassword: "secret", AdminEmail: "admin@example.com", InstanceName: "demo", SkipUp: true, SkipBuildJS: true, Contexts: []string{"mysql", "redis"}}
	}
	t.Run("fresh licensed installation", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "site")
		envFile := filepath.Join(t.TempDir(), "values")
		if err := os.WriteFile(envFile, []byte("# comment\nXF_EMAIL=file@example.com\nCUSTOM=file\nFILE_ONLY=from-file\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		opts := valid(target)
		opts.Products = []string{" addon ", "xenforo", "addon", ""}
		opts.EnvFile, opts.EnvFlags = envFile, []string{"CUSTOM=flag=value", "XF_EMAIL=flag@example.com"}
		if err := initialize.Run(context.Background(), opts); err != nil {
			t.Fatal(err)
		}
		wantProducts := []string{"xenforo", "addon"}
		if !reflect.DeepEqual(opts.Products, wantProducts) {
			t.Fatalf("products: %v", opts.Products)
		}
		meta, err := xf.ReadMetadata(target)
		if err != nil {
			t.Fatal(err)
		}
		if meta.LicenseKey != "license" || meta.InstanceName != "demo" || meta.InstalledVersion != "2.3.0" || meta.InstalledVersionID != 10 || !reflect.DeepEqual(meta.InstalledProducts, wantProducts) {
			t.Fatalf("metadata: %+v", meta)
		}
		values, err := environment.ReadEnvFile(filepath.Join(target, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		for k, want := range map[string]string{"XF_INSTANCE": "demo", "XF_EMAIL": "flag@example.com", "CUSTOM": "flag=value", "FILE_ONLY": "from-file", "XF_CONTEXTS": "mysql:redis", "XF_COOKIE_PREFIX": "demo_"} {
			if values[k] != want {
				t.Errorf("%s = %q, want %q", k, values[k], want)
			}
		}
		for path, mode := range map[string]os.FileMode{"data": 0o777, "data/nested": 0o777, "data/nested/file": 0o666, "internal_data": 0o777, "internal_data/file": 0o666} {
			info, err := os.Stat(filepath.Join(target, path))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != mode {
				t.Errorf("%s mode %o, want %o", path, info.Mode().Perm(), mode)
			}
		}
		mu.Lock()
		defer mu.Unlock()
		if !reflect.DeepEqual(downloads, map[string]int{"xenforo": 1, "addon": 1}) {
			t.Errorf("downloads: %v", downloads)
		}
	})
	t.Run("existing installation route", func(t *testing.T) {
		target := t.TempDir()
		if err := os.MkdirAll(filepath.Join(target, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "src", "XF.php"), []byte("original"), 0o644); err != nil {
			t.Fatal(err)
		}
		opts := &initialize.InitOptions{TargetPath: target, InstanceName: "existing", SkipBuildJS: true, EnvFlags: []string{"XF_TITLE=Existing title"}}
		if err := initialize.Run(context.Background(), opts); err != nil {
			t.Fatal(err)
		}
		values, err := environment.ReadEnvFile(filepath.Join(target, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if values["XF_INSTANCE"] != "existing" || values["XF_TITLE"] != "Existing title" {
			t.Fatalf("environment: %v", values)
		}
		data, err := os.ReadFile(filepath.Join(target, "src", "XF.php"))
		if err != nil || string(data) != "original" {
			t.Fatalf("existing source: %q %v", data, err)
		}
	})
	t.Run("invalid input and target protection", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			change func(*initialize.InitOptions)
			want   string
		}{
			{"missing flags", func(o *initialize.InitOptions) { o.LicenseKey = ""; o.AdminPassword = "" }, "missing required flags"},
			{"bad env", func(o *initialize.InitOptions) { o.EnvFlags = []string{"bad-key=value"} }, "invalid --env"},
			{"bad env file", func(o *initialize.InitOptions) {
				p := filepath.Join(t.TempDir(), "env")
				if err := os.WriteFile(p, []byte("# hi\nBROKEN\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				o.EnvFile = p
			}, "invalid env line 2"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				o := valid(filepath.Join(t.TempDir(), "site"))
				tc.change(o)
				err := initialize.Run(context.Background(), o)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error: %v", err)
				}
				if tc.name == "missing flags" && !clierror.IsUsage(err) {
					t.Fatalf("missing flags should be a usage error: %v", err)
				}
			})
		}
		target := t.TempDir()
		marker := filepath.Join(target, "important")
		if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := initialize.Run(context.Background(), valid(target))
		if !errors.Is(err, clierror.ErrInvalidInput) || !strings.Contains(err.Error(), "not empty") {
			t.Fatalf("error: %v", err)
		}
		data, err := os.ReadFile(marker)
		if err != nil || string(data) != "keep" {
			t.Fatalf("marker: %q %v", data, err)
		}
	})
	t.Run("API failure", func(t *testing.T) {
		mu.Lock()
		failVersions = true
		mu.Unlock()
		defer func() { mu.Lock(); failVersions = false; mu.Unlock() }()
		target := filepath.Join(t.TempDir(), "site")
		err := initialize.Run(context.Background(), valid(target))
		if err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "resolve product selections") {
			t.Fatalf("error: %v", err)
		}
		for _, name := range []string{".xf.json", "src/XF.php"} {
			if _, err := os.Stat(filepath.Join(target, name)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("%s should not exist after failed version resolution: %v", name, err)
			}
		}
	})
}
