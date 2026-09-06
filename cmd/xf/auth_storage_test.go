package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
)

// Subprocesses isolate Cobra/Viper's globals and exercise the actual command wiring.
func TestAuthStorageProcess(t *testing.T) {
	if os.Getenv("XF_AUTH_TEST_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			rootCmd.SetArgs(os.Args[i+1:])
			if err := rootCmd.ExecuteContext(t.Context()); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func authCommand(t *testing.T, configPath, token string, args ...string) (string, error) {
	t.Helper()
	commandArgs := append([]string{"-test.run=^TestAuthStorageProcess$", "--", "--config", configPath}, args...)
	cmd := exec.CommandContext(t.Context(), os.Args[0], commandArgs...)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "XF_") || (len(args) > 1 && args[0] == "auth" && args[1] == "login" && strings.HasPrefix(entry, "PATH=")) {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "XF_AUTH_TEST_HELPER=1", "XF_TOKEN="+token)
	if len(args) > 1 && args[0] == "auth" && args[1] == "login" {
		cmd.Env = append(cmd.Env, "PATH=")
	}
	output, err := cmd.CombinedOutput()
	if token != "" && strings.Contains(string(output), token) {
		t.Fatal("command exposed environment token")
	}
	return string(output), err
}

func authConfig(t *testing.T, baseURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data, err := json.Marshal(map[string]any{"auth": map[string]string{"storage": "file"}, "oauth": map[string]string{"base_url": baseURL, "client_id": "test"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEnvironmentAuthCommands(t *testing.T) {
	for _, tc := range []struct {
		name, body    string
		status        int
		authenticated any
	}{
		{"active", `{"active":true,"exp":2000000000}`, 200, true},
		{"inactive", `{"active":false}`, 200, false},
		{"unavailable", "sensitive-env-token", 500, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/customer-oauth2/introspect" {
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("token") != "sensitive-env-token" || r.Form.Get("client_id") != "test" {
					t.Error("wrong introspection credentials")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			path := authConfig(t, server.URL)
			// An unusable file proves environment authentication bypasses file reads.
			if err := os.Mkdir(filepath.Join(filepath.Dir(path), "auth.json"), 0o700); err != nil {
				t.Fatal(err)
			}
			output, err := authCommand(t, path, "sensitive-env-token", "auth", "status", "--json")
			if err != nil {
				t.Fatalf("status: %v: %s", err, output)
			}
			var status map[string]any
			if err := json.Unmarshal([]byte(output), &status); err != nil {
				t.Fatalf("JSON: %v: %s", err, output)
			}
			if status["source"] != "environment" || status["authenticated"] != tc.authenticated {
				t.Fatalf("status=%v", status)
			}
			if tc.authenticated == nil && (status["expired"] != nil || status["expires_at"] != nil) {
				t.Fatal("unknown expiry reported as known")
			}

		})
	}
}

func TestEnvironmentCredentialsAreReadOnly(t *testing.T) {
	path := authConfig(t, "http://127.0.0.1:1")
	for _, command := range []string{"login", "refresh", "logout"} {
		output, err := authCommand(t, path, "sensitive-env-token", "auth", command)
		if err == nil || !strings.Contains(output, "externally") {
			t.Fatalf("%s: %v: %s", command, err, output)
		}
	}
}

func TestFileAuthCommands(t *testing.T) {
	var revoked []string
	var revokedMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/api/customer-oauth2/token":
			if r.Form.Get("refresh_token") != "old-refresh" {
				t.Error("wrong refresh token")
			}
			fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`)
		case "/api/customer-oauth2/introspect":
			fmt.Fprint(w, `{"active":true}`)
		case "/api/customer-oauth2/revoke":
			revokedMu.Lock()
			revoked = append(revoked, r.Form.Get("token"))
			revokedMu.Unlock()
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	path := authConfig(t, server.URL)
	token := &auth.Token{AccessToken: "old-access", RefreshToken: "old-refresh", BaseURL: server.URL, ExpiresAt: time.Now().Add(time.Hour)}
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(filepath.Dir(path), "auth.json")
	if err := os.WriteFile(tokenPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"status", "refresh", "status", "logout", "logout"} {
		if command == "logout" && runtime.GOOS != "windows" {
			if err := os.Chmod(tokenPath, 0o644); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
		output, err := authCommand(t, path, "", "auth", command)
		if err != nil {
			t.Fatalf("%s: %v: %s", command, err, output)
		}
		if strings.Contains(output, "old-access") || strings.Contains(output, "new-access") || strings.Contains(output, "old-refresh") || strings.Contains(output, "new-refresh") {
			t.Fatal("exposed stored credential")
		}
		if command == "status" && !strings.Contains(output, "file") {
			t.Fatalf("missing source: %s", output)
		}
		if command == "refresh" {
			data, err := os.ReadFile(tokenPath)
			if err != nil {
				t.Fatal(err)
			}
			var refreshed auth.Token
			if err := json.Unmarshal(data, &refreshed); err != nil {
				t.Fatal(err)
			}
			if refreshed.AccessToken != "new-access" || refreshed.RefreshToken != "new-refresh" || refreshed.BaseURL != server.URL {
				t.Fatal("refresh did not persist full credentials")
			}
		}
	}
	revokedMu.Lock()
	defer revokedMu.Unlock()
	if strings.Join(revoked, ",") != "new-access,new-refresh" {
		t.Fatalf("revocations=%v", revoked)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("logout left file: %v", err)
	}
}

func TestFileLoginFailsBeforeBrowserForUnsafeStorage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions and symlinks")
	}
	for _, kind := range []string{"permissions", "symlink", "directory", "unwritable_directory"} {
		t.Run(kind, func(t *testing.T) {
			path := authConfig(t, "http://127.0.0.1:1")
			tokenPath := filepath.Join(filepath.Dir(path), "auth.json")
			switch kind {
			case "permissions":
				if err := os.WriteFile(tokenPath, []byte("secret"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(tokenPath, 0o644); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(path, tokenPath); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(tokenPath, 0o700); err != nil {
					t.Fatal(err)
				}
			case "unwritable_directory":
				if err := os.Chmod(filepath.Dir(path), 0o500); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
						t.Errorf("restore directory permissions: %v", err)
					}
				})
			}
			output, err := authCommand(t, path, "", "auth", "login", "--timeout", "1")
			if err == nil || strings.Contains(output, "Opening browser") || strings.Contains(output, "Waiting for authentication") {
				t.Fatalf("login did not fail before browser authentication: %v: %s", err, output)
			}
			if !strings.Contains(output, "authentication file") && !strings.Contains(output, "authentication directory") {
				t.Fatalf("unexpected failure: %s", output)
			}
		})
	}
}

func TestAuthStatusErrorsRemainJSON(t *testing.T) {
	for _, kind := range []string{"missing", "corrupt", "permissions", "invalid_storage", "invalid_environment", "configuration_mismatch"} {
		t.Run(kind, func(t *testing.T) {
			path := authConfig(t, "http://127.0.0.1:1")
			tokenPath := filepath.Join(filepath.Dir(path), "auth.json")
			token := ""
			reason := "invalid_credentials"
			switch kind {
			case "missing":
				reason = "not_authenticated"
			case "corrupt":
				if err := os.WriteFile(tokenPath, []byte(`{"access_token":"secret-value",`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if runtime.GOOS == "windows" {
					t.Skip("Unix permissions")
				}
				if err := os.WriteFile(tokenPath, []byte("secret-value"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(tokenPath, 0o644); err != nil {
					t.Fatal(err)
				}
			case "invalid_storage":
				if err := os.WriteFile(path, []byte(`{"auth":{"storage":"bogus"}}`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "invalid_environment":
				token = "secret-value\n"
			case "configuration_mismatch":
				reason = "configuration_mismatch"
				if err := os.WriteFile(tokenPath, []byte(`{"access_token":"secret-value","base_url":"https://different.example"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			output, err := authCommand(t, path, token, "auth", "status", "--json")
			if err != nil {
				t.Fatalf("status: %v: %s", err, output)
			}
			var status map[string]any
			if err := json.Unmarshal([]byte(output), &status); err != nil {
				t.Fatalf("not JSON: %v: %s", err, output)
			}
			if status["authenticated"] != false || status["reason"] != reason {
				t.Fatalf("status=%v", status)
			}
			if reason == "not_authenticated" && status["error"] != nil {
				t.Fatal("normal logged-out state has an error")
			}
			if strings.Contains(output, "secret-value") {
				t.Fatal("status exposed credentials")
			}
		})
	}
}
