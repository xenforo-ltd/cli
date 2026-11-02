package customerapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
)

func TestClientBehavior(t *testing.T) {
	if os.Getenv("XF_CUSTOMERAPI_TEST_CHILD") != "1" {
		// Configuration is cached for the process lifetime.
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestClientBehavior$")
		if dir := os.Getenv("GOCOVERDIR"); dir != "" {
			cmd.Args = append(cmd.Args, "-test.gocoverdir="+dir)
		}
		cmd.Env = append(os.Environ(), "XF_CUSTOMERAPI_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated client test: %v\n%s", err, output)
		}
		return
	}
	var mu sync.Mutex
	mode := "success"
	apiCalls, refreshCalls := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/customer-oauth2/token" {
			refreshCalls++
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh-secret" || r.Form.Get("client_id") != "test-client" {
				t.Errorf("refresh form: %v", r.Form)
			}
			fmt.Fprint(w, `{"access_token":"new-token","token_type":"Bearer","expires_in":3600}`)
			return
		}
		apiCalls++
		if r.Header.Get("Accept") != "application/json" || !strings.HasPrefix(r.Header.Get("User-Agent"), "xenforo-cli/") {
			t.Errorf("headers: %v", r.Header)
		}
		if mode == "success" {
			if r.URL.Path != "/api/customer-oauth2/license-versions" || r.URL.Query().Get("license_key") != "key +/&" || r.URL.Query().Get("download_id") != "add on/?" || r.Header.Get("Authorization") != "Bearer old-token" {
				t.Errorf("request: %s %v", r.URL, r.Header)
			}
			fmt.Fprint(w, `{"versions":[{"version_id":12,"version_string":"2.3","release_date":1700000000,"stable":true}]}`)
			return
		}
		if mode == "retry" && r.Header.Get("Authorization") == "Bearer new-token" {
			fmt.Fprint(w, `{"licenses":[{"license_key":"license","is_valid":true}]}`)
			return
		}
		if mode == "malformed" {
			fmt.Fprint(w, `{broken`)
			return
		}
		if mode == "empty-errors" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"errors":[]}`)
			return
		}
		if mode == "server-error" {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"errors":[{"message":"rejected"}]}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"auth":{"storage":"file"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XF_AUTH_STORAGE", "file")
	t.Setenv("XF_OAUTH_BASE_URL", server.URL)
	t.Setenv("XF_OAUTH_CLIENT_ID", "test-client")
	t.Setenv("XF_TOKEN", "")
	if err := config.Init(configPath); err != nil {
		t.Fatal(err)
	}
	store, err := auth.NewWritableStore()
	if err != nil {
		t.Fatal(err)
	}
	save := func(t *testing.T) {
		t.Helper()
		if err := store.SaveToken(&auth.Token{AccessToken: "old-token", RefreshToken: "refresh-secret", BaseURL: server.URL}); err != nil {
			t.Fatal(err)
		}
	}
	reset := func(modeValue string) {
		mu.Lock()
		defer mu.Unlock()
		mode = modeValue
		apiCalls, refreshCalls = 0, 0
	}
	counts := func() (int, int) {
		mu.Lock()
		defer mu.Unlock()
		return apiCalls, refreshCalls
	}
	save(t)
	client, err := customerapi.NewClient()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("successful request and download URL", func(t *testing.T) {
		save(t)
		reset("success")
		versions, err := client.GetLicenseVersions(t.Context(), "key +/&", "add on/?")
		if err != nil || len(versions.Versions) != 1 || versions.Versions[0].VersionID != 12 || versions.Versions[0].ReleaseDate.Unix() != 1700000000 {
			t.Fatalf("versions: %#v %v", versions, err)
		}
		downloadURL, err := url.Parse(client.GetDownloadURL("key +/&", "add on/?", 42))
		if err != nil {
			t.Fatal(err)
		}
		if downloadURL.Query().Get("license_key") != "key +/&" || downloadURL.Query().Get("download_id") != "add on/?" || downloadURL.Query().Get("version_id") != "42" {
			t.Fatalf("download URL: %s", downloadURL)
		}
	})

	t.Run("malformed response", func(t *testing.T) {
		save(t)
		reset("malformed")
		if _, err := client.GetLicenses(t.Context()); err == nil || !strings.Contains(err.Error(), "decode") {
			t.Fatalf("malformed API response: %v", err)
		}
	})

	t.Run("error without details", func(t *testing.T) {
		save(t)
		reset("empty-errors")
		_, err := client.GetLicenses(t.Context())
		if err == nil || strings.Contains(err.Error(), "%!w") {
			t.Fatalf("empty error details: %v", err)
		}
		if !strings.Contains(err.Error(), "API error") || !strings.Contains(err.Error(), "400") {
			t.Fatalf("missing API error context: %v", err)
		}
	})

	t.Run("server error", func(t *testing.T) {
		save(t)
		reset("server-error")
		if _, err := client.GetLicenses(t.Context()); err == nil || !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "service unavailable") {
			t.Fatalf("API server error: %v", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		save(t)
		reset("success")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := client.GetLicenses(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled API request: %v", err)
		}
	})

	t.Run("expired token refresh and retry", func(t *testing.T) {
		save(t)
		reset("retry")
		licenses, err := client.GetLicenses(t.Context())
		if err != nil || len(licenses) != 1 || licenses[0].LicenseKey != "license" {
			t.Fatalf("licenses: %#v %v", licenses, err)
		}
		got, err := store.LoadToken()
		if err != nil || got.AccessToken != "new-token" || got.RefreshToken != "refresh-secret" {
			t.Fatalf("persisted token: %#v %v", got, err)
		}
		if api, refresh := counts(); api != 2 || refresh != 1 {
			t.Errorf("retry counts: API %d refresh %d", api, refresh)
		}
	})

	t.Run("concurrent requests share a token refresh", func(t *testing.T) {
		save(t)
		reset("retry")
		results := make(chan error, 8)
		for range cap(results) {
			go func() {
				licenses, err := client.GetLicenses(t.Context())
				if err == nil && len(licenses) != 1 {
					err = fmt.Errorf("got %d licenses, want 1", len(licenses))
				}
				results <- err
			}()
		}
		for range cap(results) {
			if err := <-results; err != nil {
				t.Error(err)
			}
		}
		if _, refresh := counts(); refresh != 1 {
			t.Errorf("concurrent requests refreshed %d times, want once", refresh)
		}
	})

	t.Run("refresh rejection is not retried indefinitely", func(t *testing.T) {
		save(t)
		reset("reject")
		if _, err := client.GetLicenses(t.Context()); err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Fatalf("retry rejection: %v", err)
		}
		if api, refresh := counts(); api != 2 || refresh != 1 {
			t.Errorf("bounded retry counts: API %d refresh %d", api, refresh)
		}
	})

	t.Run("environment token is rejected without refresh", func(t *testing.T) {
		reset("reject")
		t.Setenv("XF_TOKEN", "external-token")
		external, err := customerapi.NewClient()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := external.GetLicenses(t.Context()); err == nil || !strings.Contains(err.Error(), "XF_TOKEN") {
			t.Fatalf("external rejection: %v", err)
		}
		if api, refresh := counts(); api != 1 || refresh != 0 {
			t.Errorf("external counts: API %d refresh %d", api, refresh)
		}
	})
}
