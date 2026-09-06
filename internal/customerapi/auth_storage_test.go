package customerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
)

func TestClientCredentialSources(t *testing.T) {
	if os.Getenv("XF_CUSTOMERAPI_TEST_HELPER") != "1" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestClientCredentialSources$")
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "XF_") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		cmd.Env = append(cmd.Env, "XF_CUSTOMERAPI_TEST_HELPER=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("credential integration test: %v\n%s", err, output)
		}
		return
	}

	var refreshCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/customer-oauth2/token" {
			refreshCalls.Add(1)
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("refresh_token") != "refresh-secret" {
				t.Error("wrong refresh token")
			}
			fmt.Fprint(w, `{"access_token":"refreshed","token_type":"Bearer","expires_in":3600}`)
			return
		}
		switch r.Header.Get("Authorization") {
		case "Bearer env-valid", "Bearer refreshed":
			fmt.Fprint(w, `{"licenses":[]}`)
		case "Bearer env-invalid", "Bearer expired":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			t.Error("incorrect Authorization header")
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	data, err := json.Marshal(map[string]any{"auth": map[string]string{"storage": "file"}, "oauth": map[string]string{"base_url": server.URL, "client_id": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XF_AUTH_STORAGE", "file")
	t.Setenv("XF_OAUTH_BASE_URL", server.URL)
	if err := config.Init(cfgPath); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"env-valid", "env-invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("XF_TOKEN", value)
			client, err := NewClient()
			if err != nil {
				t.Fatal(err)
			}
			access, err := client.GetAccessToken()
			if err != nil || access != value {
				t.Fatal("download access token did not use environment")
			}
			_, err = client.GetLicenses(t.Context())
			if value == "env-valid" && err != nil {
				t.Fatal(err)
			}
			if value == "env-invalid" && (err == nil || !strings.Contains(err.Error(), "replace")) {
				t.Fatalf("rejected token: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), value) {
				t.Fatal("error exposed token")
			}
			if refreshCalls.Load() != 0 {
				t.Fatal("environment token attempted refresh")
			}
			if _, err := os.Stat(filepath.Join(dir, "auth.json")); !os.IsNotExist(err) {
				t.Fatal("environment token persisted")
			}
		})
	}
	t.Setenv("XF_TOKEN", "")
	store, err := auth.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	token := &auth.Token{AccessToken: "expired", RefreshToken: "refresh-secret", BaseURL: server.URL, ExpiresAt: time.Now().Add(-time.Hour)}
	if err := store.SaveToken(token); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetLicenses(t.Context()); err != nil {
		t.Fatal(err)
	}
	if refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls=%d", refreshCalls.Load())
	}
	refreshed, err := store.LoadToken()
	if err != nil || refreshed.AccessToken != "refreshed" || refreshed.RefreshToken != "refresh-secret" {
		t.Fatalf("file refresh failed: %v", err)
	}
	access, err := client.GetAccessToken()
	if err != nil || access != "refreshed" {
		t.Fatal("download token did not observe file refresh")
	}
	token.BaseURL = "https://different.example"
	if err := store.SaveToken(token); err != nil {
		t.Fatal(err)
	}
	if _, err := NewClient(); err == nil || !strings.Contains(err.Error(), "different configuration") {
		t.Fatalf("configuration mismatch accepted: %v", err)
	}
}
