package auth_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
)

func TestStoreBehavior(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"auth":{"storage":"file"},"oauth":{"base_url":"http://configured.invalid"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XF_AUTH_STORAGE", "file")
	t.Setenv("XF_OAUTH_BASE_URL", "http://configured.invalid")
	t.Setenv("XF_TOKEN", "")
	if err := config.Init(path); err != nil {
		t.Fatal(err)
	}
	store, err := auth.NewWritableStore()
	if err != nil {
		t.Fatal(err)
	}
	file := store.(*auth.FileStore)
	if file.Source() != "file" || file.Path() != filepath.Join(dir, "auth.json") {
		t.Fatalf("unexpected file store: %s %s", file.Source(), file.Path())
	}
	if _, err := file.LoadToken(); !errors.Is(err, auth.ErrAuthRequired) {
		t.Fatalf("missing token: %v", err)
	}
	if err := file.PrepareLogin(); err != nil {
		t.Fatal(err)
	}
	token := &auth.Token{AccessToken: "stored", RefreshToken: "refresh", BaseURL: "http://configured.invalid"}
	t.Run("lifecycle and destination", func(t *testing.T) {
		if err := file.SaveToken(token); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(file.Path())
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("permissions: %o", info.Mode().Perm())
		}
		got, err := auth.RequireAuthFrom(file)
		if err != nil || got.AccessToken != token.AccessToken || got.RefreshToken != token.RefreshToken {
			t.Fatalf("load: %#v, %v", got, err)
		}
		if err := file.SaveToken(&auth.Token{AccessToken: "other", BaseURL: "http://elsewhere.invalid"}); err != nil {
			t.Fatal(err)
		}
		if _, err := auth.RequireAuthFrom(file); !errors.Is(err, auth.ErrConfigMismatch) {
			t.Fatalf("mismatch: %v", err)
		}
	})
	t.Run("insecure file and logout", func(t *testing.T) {
		if err := file.DeleteToken(); err != nil {
			t.Fatal(err)
		}
		if err := file.SaveToken(token); err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(file.Path(), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := file.LoadToken(); !errors.Is(err, auth.ErrInvalidInput) {
				t.Fatalf("insecure load: %v", err)
			}
			if err := file.PrepareLogin(); !errors.Is(err, auth.ErrInvalidInput) {
				t.Fatalf("insecure login: %v", err)
			}
			if got, err := file.LoadTokenForLogout(); err != nil || got.AccessToken != "stored" {
				t.Fatalf("logout load: %#v, %v", got, err)
			}
		}
		if err := file.DeleteToken(); err != nil {
			t.Fatal(err)
		}
		if err := file.DeleteToken(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid credential files", func(t *testing.T) {
		for name, data := range map[string]string{
			"malformed":          `{"access_token":`,
			"empty access token": `{"access_token":"","base_url":"http://configured.invalid"}`,
		} {
			t.Run(name, func(t *testing.T) {
				if err := os.WriteFile(file.Path(), []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := file.LoadToken(); !errors.Is(err, auth.ErrInvalidInput) {
					t.Fatalf("invalid credentials: %v", err)
				}
			})
		}
		if err := file.DeleteToken(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("symlink rejection", func(t *testing.T) {
		target := filepath.Join(dir, "target")
		if err := os.WriteFile(target, []byte("untouched"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, file.Path()); err != nil {
			if runtime.GOOS == "windows" && (errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.Errno(1314))) {
				t.Skipf("symlink privilege unavailable: %v", err)
			}
			t.Fatal(err)
		}
		for name, run := range map[string]func() error{
			"load":   func() error { _, err := file.LoadToken(); return err },
			"logout": func() error { _, err := file.LoadTokenForLogout(); return err },
			"save":   func() error { return file.SaveToken(token) },
			"delete": file.DeleteToken,
			"login":  file.PrepareLogin,
		} {
			if err := run(); !errors.Is(err, auth.ErrInvalidInput) {
				t.Errorf("symlink %s: %v", name, err)
			}
		}
		data, err := os.ReadFile(target)
		if err != nil || string(data) != "untouched" {
			t.Fatalf("symlink target: %q %v", data, err)
		}
	})
	t.Run("environment precedence and validation", func(t *testing.T) {
		if err := os.Remove(file.Path()); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if err := file.SaveToken(token); err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{"bad token", "bad\nsecret", "bad\tsecret", "bad\u2003token"} {
			t.Setenv("XF_TOKEN", raw)
			if _, err := auth.NewStore(); !errors.Is(err, auth.ErrInvalidInput) {
				t.Errorf("invalid external token %q: %v", raw, err)
			}
		}
		t.Setenv("XF_TOKEN", "external-token")
		external, err := auth.NewStore()
		if err != nil {
			t.Fatal(err)
		}
		got, err := auth.RequireAuthFrom(external)
		if err != nil || external.Source() != "environment" || got.AccessToken != "external-token" || !got.External || got.BaseURL != token.BaseURL {
			t.Fatalf("external: %#v %v", got, err)
		}
		got.AccessToken = "changed"
		got, err = external.LoadToken()
		if err != nil || got.AccessToken != "external-token" {
			t.Fatalf("external copy: %#v %v", got, err)
		}
		if external.SaveToken(token) == nil || external.DeleteToken() == nil {
			t.Fatal("external store was writable")
		}
		if _, err := auth.NewWritableStore(); err == nil || !strings.Contains(err.Error(), "XF_TOKEN") {
			t.Fatalf("writable external: %v", err)
		}
	})
}

func TestOAuthBehavior(t *testing.T) {
	var challenge string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.Form.Get("client_id") != "client" {
			t.Errorf("client_id: %s", r.Form.Get("client_id"))
		}
		switch r.URL.Path {
		case "/api/customer-oauth2/token":
			switch r.Form.Get("grant_type") {
			case "authorization_code":
				if r.Form.Get("code") != "code" || r.Form.Get("code_verifier") == "" || r.Form.Get("redirect_uri") != "http://localhost/callback" {
					t.Errorf("exchange: %v", r.Form)
				}
				hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
				if actual := base64.RawURLEncoding.EncodeToString(hash[:]); actual != challenge {
					t.Errorf("PKCE challenge = %q, want %q", challenge, actual)
				}
				fmt.Fprint(w, `{"access_token":"access","refresh_token":"initial","token_type":"Bearer","expires_in":3600}`)
			case "refresh_token":
				if r.Form.Get("refresh_token") == "fail" {
					http.Error(w, "invalid grant", http.StatusBadRequest)
					return
				}
				fmt.Fprint(w, `{"access_token":"renewed","token_type":"Bearer","expires_in":3600}`)
			default:
				t.Errorf("grant: %v", r.Form)
			}
		case "/api/customer-oauth2/introspect":
			if r.Form.Get("token") != "access" {
				t.Errorf("introspect: %v", r.Form)
			}
			fmt.Fprint(w, `{"active":true,"username":"alice"}`)
		case "/api/customer-oauth2/revoke":
			if r.Form.Get("token") != "access" {
				t.Errorf("revoke: %v", r.Form)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := auth.NewOAuthClient(&config.OAuthConfig{BaseURL: server.URL, ClientID: "client", Scopes: []string{"licenses:read"}})
	pkce, err := auth.GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(client.AuthorizationURL(pkce, "http://localhost/callback"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/customer-oauth/authorize" || u.Query().Get("state") != pkce.State || u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") == "" {
		t.Fatalf("authorization URL: %s", u)
	}
	challenge = u.Query().Get("code_challenge")
	token, err := client.ExchangeCode(context.Background(), "code", pkce, "http://localhost/callback")
	if err != nil || token.AccessToken != "access" || token.IsExpired() || token.BaseURL != server.URL {
		t.Fatalf("exchange: %#v %v", token, err)
	}
	refreshed, err := client.RefreshToken(context.Background(), "initial")
	if err != nil || refreshed.AccessToken != "renewed" || refreshed.RefreshToken != "initial" {
		t.Fatalf("refresh: %#v %v", refreshed, err)
	}
	if _, err := client.RefreshToken(context.Background(), "fail"); err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid grant") {
		t.Fatalf("refresh error: %v", err)
	}
	introspection, err := client.IntrospectToken(context.Background(), "access")
	if err != nil || !introspection.Active || introspection.Username != "alice" {
		t.Fatalf("introspection: %#v %v", introspection, err)
	}
	if err := client.RevokeToken(context.Background(), "access"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.RefreshToken(ctx, "initial"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
