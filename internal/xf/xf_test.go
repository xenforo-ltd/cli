package xf_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/xenforo-ltd/cli/internal/xf"
)

func writeFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryPrefersAncestorAndValidatesFallback(t *testing.T) {
	root := t.TempDir()
	ancestor := filepath.Join(root, "ancestor")
	fallback := filepath.Join(root, "fallback")
	writeFile(t, filepath.Join(ancestor, "src", "XF.php"), "")
	writeFile(t, filepath.Join(fallback, "src", "XF.php"), "")
	t.Setenv("XF_DIR", fallback)
	got, err := xf.GetXenForoDir(filepath.Join(ancestor, "nested", "dir"))
	if err != nil || got != ancestor {
		t.Fatalf("ancestor = %q, %v", got, err)
	}
	got, err = xf.GetXenForoDir(filepath.Join(root, "other"))
	if err != nil || got != fallback {
		t.Fatalf("fallback = %q, %v", got, err)
	}
	t.Setenv("XF_DIR", filepath.Join(root, "invalid"))
	if _, err := xf.GetXenForoDir(filepath.Join(root, "other")); err == nil {
		t.Fatal("accepted invalid XF_DIR")
	}
}

func TestMetadataLifecycle(t *testing.T) {
	dir := t.TempDir()
	if _, err := xf.ReadMetadata(dir); !errors.Is(err, xf.ErrMetadataNotFound) {
		t.Fatalf("missing metadata: %v", err)
	}
	meta := &xf.Metadata{LicenseKey: "secret", InstanceName: "forum", InstalledProducts: []string{"XF", "XFMG"}}
	if err := xf.WriteMetadata(dir, meta); err != nil {
		t.Fatal(err)
	}
	if meta.CreatedAt.IsZero() || meta.UpdatedAt.IsZero() {
		t.Fatalf("timestamps: %+v", meta)
	}
	info, err := os.Stat(filepath.Join(dir, ".xf.json"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions: %v", info.Mode().Perm())
	}
	if err := xf.UpdateMetadataVersion(dir, &xf.Version{String: "2.3.8", ID: 2030871}); err != nil {
		t.Fatal(err)
	}
	got, err := xf.ReadMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.LicenseKey != meta.LicenseKey || got.InstanceName != meta.InstanceName || !reflect.DeepEqual(got.InstalledProducts, meta.InstalledProducts) || !got.CreatedAt.Equal(meta.CreatedAt) || got.UpdatedAt.Before(meta.UpdatedAt) || got.InstalledVersion != "2.3.8" || got.InstalledVersionID != 2030871 {
		t.Fatalf("updated metadata: %+v", got)
	}
	writeFile(t, filepath.Join(dir, ".xf.json"), "not json")
	if err := xf.UpdateMetadataVersion(dir, &xf.Version{String: "2.3.9"}); err == nil {
		t.Fatal("accepted malformed metadata")
	}
	if b, err := os.ReadFile(filepath.Join(dir, ".xf.json")); err != nil || string(b) != "not json" {
		t.Fatalf("malformed metadata overwritten: %q %v", b, err)
	}
}

func TestDetectVersionFilesystemContracts(t *testing.T) {
	php := "<?php\npublic static $version = '2.3.8';\npublic static $versionId = 2030871;\n"
	manifest := `{"version_string":"2.3.8","version_id":2030871}`
	files := fstest.MapFS{"src/XF.php": &fstest.MapFile{Data: []byte(php)}}
	got, err := xf.DetectVersionFS(files)
	if err != nil || got.String != "2.3.8" || got.ID != 2030871 {
		t.Fatalf("PHP only: %+v, %v", got, err)
	}
	files["src/addons/XF/addon.json"] = &fstest.MapFile{Data: []byte(manifest)}
	got, err = xf.DetectVersionFS(files)
	if err != nil || got.String != "2.3.8" || got.ID != 2030871 {
		t.Fatalf("matching manifest: %+v, %v", got, err)
	}
	files["src/addons/XF/addon.json"] = &fstest.MapFile{Data: []byte(`{"version_string":"2.3.9","version_id":2030971}`)}
	if _, err := xf.DetectVersionFS(files); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("mismatch: %v", err)
	}
	files["src/addons/XF/addon.json"] = &fstest.MapFile{Data: []byte(`not json`)}
	if _, err := xf.DetectVersionFS(files); err == nil {
		t.Fatal("accepted malformed manifest")
	}
	if _, err := xf.DetectVersionFS(fstest.MapFS{}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing XF.php: %v", err)
	}
}
