package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	storeauth "github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/clierror"
	command "github.com/xenforo-ltd/cli/internal/commands/auth"
	"github.com/xenforo-ltd/cli/internal/config"
)

var (
	serverURL      string
	credentialPath string
	revocationMu   sync.Mutex
	revocations    []string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "xf-auth-command-test-")
	if err != nil {
		panic(err)
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "XF_") {
			_ = os.Unsetenv(key)
		}
	}
	_ = os.Setenv("HOME", dir)
	_ = os.Setenv("XDG_CONFIG_HOME", dir)
	_ = os.Setenv("XDG_CACHE_HOME", dir)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.ParseForm() != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/api/customer-oauth2/introspect":
			switch r.Form.Get("token") {
			case "env-rejected":
				fmt.Fprint(w, `{"active":false}`)
			case "env-active":
				fmt.Fprint(w, `{"active":true,"username":"alice","scope":"licenses:read","iat":1700000000,"exp":4102444800}`)
			default:
				http.Error(w, "secret server response", http.StatusServiceUnavailable)
			}
		case "/api/customer-oauth2/token":
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("client_id") != "test-client" {
				http.Error(w, "bad grant", http.StatusBadRequest)
				return
			}
			if r.Form.Get("refresh_token") == "refresh-ok" {
				fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"scope":"new-scope"}`)
				return
			}
			http.Error(w, "refresh denied", http.StatusUnauthorized)
		case "/api/customer-oauth2/revoke":
			revocationMu.Lock()
			revocations = append(revocations, r.Form.Get("token"))
			revocationMu.Unlock()
			if strings.HasPrefix(r.Form.Get("token"), "revoke-denied") {
				http.Error(w, "revocation unavailable", http.StatusServiceUnavailable)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	credentialPath = filepath.Join(dir, "auth.json")
	configPath := filepath.Join(dir, "config.json")
	data, _ := json.Marshal(map[string]any{"auth": map[string]string{"storage": "file"}, "oauth": map[string]string{"base_url": serverURL, "client_id": "test-client"}})
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		panic(err)
	}
	if err := config.Init(configPath); err != nil {
		panic(err)
	}
	if _, err := config.Load(); err != nil {
		panic(err)
	}
	code := m.Run()
	server.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func cleanCredentials(t *testing.T) {
	t.Helper()
	if err := os.Remove(credentialPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(credentialPath) })
	t.Setenv("XF_TOKEN", "")
}

func writeToken(t *testing.T, token storeauth.Token) []byte {
	t.Helper()
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func token(refresh string) storeauth.Token {
	return storeauth.Token{AccessToken: "access-secret", RefreshToken: refresh, TokenType: "Bearer", BaseURL: serverURL, IssuedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour)}
}

func status(t *testing.T) (map[string]any, string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "status-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old }()
	err = command.Status(context.Background(), command.StatusOptions{JSON: true})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid status JSON %q: %v", data, err)
	}
	return got, string(data)
}

func TestStatusEnvironmentJSON(t *testing.T) {
	cleanCredentials(t)
	for _, tc := range []struct {
		name, value string
		want        map[string]any
		wantError   bool
	}{
		{"unknown", "env-secret", map[string]any{"authenticated": nil, "source": "environment", "server_valid": nil, "expires_at": nil, "issued_at": nil, "expired": nil}, true},
		{"rejected", "env-rejected", map[string]any{"authenticated": false, "source": "environment", "server_valid": false, "expires_at": nil, "issued_at": nil, "expired": nil, "scope": "", "username": ""}, false},
		{"active", "env-active", map[string]any{"authenticated": true, "source": "environment", "server_valid": true, "expires_at": time.Unix(4102444800, 0).Format(time.RFC3339), "issued_at": time.Unix(1700000000, 0).Format(time.RFC3339), "expired": false, "scope": "licenses:read", "username": "alice"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XF_TOKEN", tc.value)
			got, raw := status(t)
			errorText, hasError := got["error"].(string)
			delete(got, "error")
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("status = %#v, want %#v", got, tc.want)
			}
			if hasError != tc.wantError || (tc.wantError && errorText == "") {
				t.Errorf("unexpected error field: %s", raw)
			}
			if strings.Contains(raw, tc.value) || strings.Contains(raw, "secret server response") {
				t.Errorf("status leaked credentials or server body: %s", raw)
			}
		})
	}
}

func TestStatusUnavailableJSON(t *testing.T) {
	cleanCredentials(t)
	tests := []struct {
		name, reason string
		prepare      func(*testing.T)
	}{
		{"logged out", "not_authenticated", func(t *testing.T) {}},
		{"malformed", "invalid_credentials", func(t *testing.T) {
			if err := os.WriteFile(credentialPath, []byte(`{"access_token":"secret-malformed`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"mismatch", "configuration_mismatch", func(t *testing.T) {
			tok := token("refresh-secret")
			tok.BaseURL = "http://different.invalid"
			writeToken(t, tok)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cleanCredentials(t)
			tc.prepare(t)
			got, raw := status(t)
			if got["authenticated"] != false || got["expired"] != nil || got["source"] != "file" || got["reason"] != tc.reason {
				t.Errorf("unexpected status: %s", raw)
			}
			if _, ok := got["expired"]; !ok {
				t.Error("expired key missing")
			}
			if _, ok := got["error"]; ok != (tc.reason != "not_authenticated") {
				t.Errorf("unexpected error field: %s", raw)
			}
			for _, secret := range []string{"secret-malformed", "access-secret", "refresh-secret"} {
				if strings.Contains(raw, secret) {
					t.Errorf("status leaked %q: %s", secret, raw)
				}
			}
		})
	}
}

func TestStatusStoredTokenJSON(t *testing.T) {
	cleanCredentials(t)
	tok := token("refresh-secret")
	tok.Scope = "licenses:read"
	writeToken(t, tok)
	got, raw := status(t)
	want := map[string]any{
		"authenticated": true, "source": "file", "storage_path": credentialPath,
		"expired": false, "scope": tok.Scope,
		"issued_at": tok.IssuedAt.Format(time.RFC3339), "expires_at": tok.ExpiresAt.Format(time.RFC3339),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("status = %#v, want %#v", got, want)
	}
	if strings.Contains(raw, tok.AccessToken) || strings.Contains(raw, tok.RefreshToken) || strings.Contains(raw, "secret server response") {
		t.Errorf("status leaked credentials or server body: %s", raw)
	}
}

func TestStatusMalformedEnvironmentToken(t *testing.T) {
	cleanCredentials(t)
	t.Setenv("XF_TOKEN", "secret with whitespace")
	got, raw := status(t)
	if got["authenticated"] != false || got["expired"] != nil || got["reason"] != "invalid_credentials" {
		t.Errorf("unexpected status: %s", raw)
	}
	if _, ok := got["expired"]; !ok {
		t.Error("expired key missing")
	}
	if strings.Contains(raw, "secret with whitespace") {
		t.Errorf("status leaked environment token: %s", raw)
	}
}

func TestRefreshFileCredentials(t *testing.T) {
	cleanCredentials(t)
	t.Run("failure preserves file", func(t *testing.T) {
		before := writeToken(t, token("refresh-denied"))
		err := command.Refresh(context.Background())
		if err == nil || !strings.Contains(err.Error(), "failed to refresh token") {
			t.Fatalf("Refresh error = %v", err)
		}
		after, err := os.ReadFile(credentialPath)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(after, before) {
			t.Error("failed refresh changed credentials")
		}
	})
	t.Run("success persists rotated access and retains refresh", func(t *testing.T) {
		writeToken(t, token("refresh-ok"))
		if err := command.Refresh(context.Background()); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(credentialPath)
		if err != nil {
			t.Fatal(err)
		}
		var got storeauth.Token
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if got.AccessToken != "new-access" || got.RefreshToken != "refresh-ok" || got.BaseURL != serverURL || got.Scope != "new-scope" || time.Until(got.ExpiresAt) < 50*time.Minute {
			t.Errorf("unexpected persisted token: %#v", got)
		}
	})
	t.Run("missing refresh hint", func(t *testing.T) {
		before := writeToken(t, token(""))
		err := command.Refresh(context.Background())
		if err == nil || !strings.Contains(err.Error(), "no refresh token") || !strings.Contains(clierror.HintOf(err), "xf auth login") {
			t.Fatalf("error = %v, hint = %q", err, clierror.HintOf(err))
		}
		after, readErr := os.ReadFile(credentialPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !reflect.DeepEqual(before, after) {
			t.Error("missing refresh changed credentials")
		}
	})
}

func TestLogoutFileCredentials(t *testing.T) {
	cleanCredentials(t)
	t.Run("revokes both", func(t *testing.T) {
		revocationMu.Lock()
		revocations = nil
		revocationMu.Unlock()
		writeToken(t, token("refresh-secret"))
		if err := command.Logout(context.Background()); err != nil {
			t.Fatal(err)
		}
		revocationMu.Lock()
		got := append([]string(nil), revocations...)
		revocationMu.Unlock()
		sort.Strings(got)
		if !reflect.DeepEqual(got, []string{"access-secret", "refresh-secret"}) {
			t.Errorf("revocations = %v", got)
		}
		if _, err := os.Stat(credentialPath); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("credentials still exist: %v", err)
		}
	})
	t.Run("failed revocations still delete", func(t *testing.T) {
		revocationMu.Lock()
		revocations = nil
		revocationMu.Unlock()
		tok := token("revoke-denied-refresh")
		tok.AccessToken = "revoke-denied-access"
		writeToken(t, tok)
		if err := command.Logout(context.Background()); err != nil {
			t.Fatal(err)
		}
		revocationMu.Lock()
		got := append([]string(nil), revocations...)
		revocationMu.Unlock()
		sort.Strings(got)
		if !reflect.DeepEqual(got, []string{"revoke-denied-access", "revoke-denied-refresh"}) {
			t.Errorf("revocations = %v", got)
		}
		if _, err := os.Stat(credentialPath); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("credentials still exist: %v", err)
		}
	})
}

func TestEnvironmentLifecycleRejectsMutations(t *testing.T) {
	cleanCredentials(t)
	before := writeToken(t, token("refresh-secret"))
	t.Setenv("XF_TOKEN", "external-secret")
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"login", func() error { return command.Login(context.Background(), command.LoginOptions{Timeout: 1}) }},
		{"refresh", func() error { return command.Refresh(context.Background()) }},
		{"logout", func() error { return command.Logout(context.Background()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), "XF_TOKEN manages authentication externally") || strings.Contains(err.Error(), "external-secret") {
				t.Fatalf("error = %v", err)
			}
			after, err := os.ReadFile(credentialPath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Error("external lifecycle changed stored credentials")
			}
		})
	}
}
