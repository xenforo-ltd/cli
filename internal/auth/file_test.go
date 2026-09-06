package auth

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fileToken() *Token {
	return &Token{AccessToken: "access-secret", RefreshToken: "refresh-secret", TokenType: "Bearer", BaseURL: "https://example.com", ExpiresAt: time.Now().UTC().Add(time.Hour), IssuedAt: time.Now().UTC(), Scope: "licenses:read"}
}

func TestFileStoreLifecycle(t *testing.T) {
	store := &FileStore{path: filepath.Join(t.TempDir(), "xf", "auth.json")}
	if _, err := store.LoadToken(); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("missing file: %v", err)
	}
	token := fileToken()
	if err := store.SaveToken(token); err != nil {
		t.Fatal(err)
	}
	old, err := os.Open(store.path)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	for _, access := range []string{"access-secret", "refreshed-secret"} {
		token.AccessToken = access
		if err := store.SaveToken(token); err != nil {
			t.Fatal(err)
		}
		got, err := store.LoadToken()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, token) {
			t.Fatal("token metadata did not survive save/load")
		}
		info, err := os.Stat(store.path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("mode=%o", info.Mode().Perm())
		}
	}
	if runtime.GOOS != "windows" {
		data := make([]byte, 4096)
		n, err := old.Read(data)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data[:n]), "access-secret") {
			t.Fatal("save modified original inode instead of replacing it")
		}
	}
	entries, err := os.ReadDir(filepath.Dir(store.path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary file leak: %v", err)
	}
	for range 2 {
		if err := store.DeleteToken(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.LoadToken(); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("deleted file: %v", err)
	}
}

func TestFileStoreRejectsUnsafeFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions and symlinks")
	}
	for _, kind := range []string{"symlink", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			store := &FileStore{path: filepath.Join(dir, "auth.json")}
			switch kind {
			case "symlink":
				target := filepath.Join(dir, "target")
				if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, store.path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(store.path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			_, loadErr := store.LoadToken()
			for _, err := range []error{loadErr, store.PrepareLogin(), store.SaveToken(fileToken()), store.DeleteToken()} {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("unsafe file accepted: %v", err)
				}
			}
		})
	}
}

func TestFileStoreInvalidData(t *testing.T) {
	store := &FileStore{path: filepath.Join(t.TempDir(), "auth.json")}
	for _, data := range []string{`{"access_token":"secret"`, `{"access_token": "secret"}`, `null`} {
		if err := os.WriteFile(store.path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := store.LoadToken()
		if !errors.Is(err, ErrInvalidInput) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid file error: %v", err)
		}
	}
	for _, token := range []*Token{nil, {}, {AccessToken: "secret", BaseURL: "https://example.com", External: true}} {
		if err := store.SaveToken(token); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid save: %v", err)
		}
	}
}

func TestFailedFileSavePreservesCredentials(t *testing.T) {
	store := &FileStore{path: filepath.Join(t.TempDir(), "auth.json")}
	token := fileToken()
	if err := store.SaveToken(token); err != nil {
		t.Fatal(err)
	}
	invalid := *token
	invalid.ExpiresAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.SaveToken(&invalid); err == nil {
		t.Fatal("expected encoding failure")
	}
	got, err := store.LoadToken()
	if err != nil || !reflect.DeepEqual(got, token) {
		t.Fatalf("failed save damaged credentials: %v", err)
	}
}

func TestFileStoreCanRevokeAndDeleteInsecureRegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions")
	}
	store := &FileStore{path: filepath.Join(t.TempDir(), "auth.json")}
	token := fileToken()
	if err := store.SaveToken(token); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.path, 0o644); err != nil {
		t.Fatal(err)
	}
	_, loadErr := store.LoadToken()
	for _, err := range []error{loadErr, store.PrepareLogin(), store.SaveToken(token)} {
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("insecure file accepted for normal use: %v", err)
		}
	}
	got, err := store.LoadTokenForLogout()
	if err != nil || !reflect.DeepEqual(got, token) {
		t.Fatalf("cannot read token for revocation: %v", err)
	}
	if err := store.DeleteToken(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("insecure credentials remain: %v", err)
	}
}

func TestFileStoreLoginPreflight(t *testing.T) {
	store := &FileStore{path: filepath.Join(t.TempDir(), "xf", "auth.json")}
	if err := store.PrepareLogin(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(store.path))
	if err != nil || len(entries) != 0 {
		t.Fatalf("preflight left files: %v", err)
	}
	if err := store.SaveToken(fileToken()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareLogin(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(store.path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("preflight changed credentials: %v", err)
	}
}
