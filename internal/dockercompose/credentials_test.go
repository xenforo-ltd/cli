package dockercompose

import (
	"os"
	"path/filepath"
	"testing"
)

// newRunnerWithEnv builds a Runner whose .env contains the given contents.
func newRunnerWithEnv(t *testing.T, env string) *Runner {
	t.Helper()

	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o750); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src", "XF.php"), []byte("<?php"), 0o600); err != nil {
		t.Fatalf("write XF.php: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n"), 0o600); err != nil {
		t.Fatalf("write compose.yaml: %v", err)
	}

	if env != "" {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
			t.Fatalf("write .env: %v", err)
		}
	}

	runner, err := NewRunner(dir)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	return runner
}

// TestDatabaseCredentialsUseComposeVariables covers the variables the compose
// files actually define. compose.mysql.yaml and compose.postgres.yaml both read
// XF_DB_USER and XF_DB_PASSWORD, so those are what .env will contain.
func TestDatabaseCredentialsUseComposeVariables(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		wantUser     string
		wantPassword string
	}{
		{
			name:         "defaults when unset",
			env:          "",
			wantUser:     "xf",
			wantPassword: "password",
		},
		{
			name:         "reads XF_DB_USER and XF_DB_PASSWORD",
			env:          "XF_DB_USER=custom\nXF_DB_PASSWORD=s3cret\n",
			wantUser:     "custom",
			wantPassword: "s3cret",
		},
		{
			name:         "user only",
			env:          "XF_DB_USER=custom\n",
			wantUser:     "custom",
			wantPassword: "password",
		},
		{
			name:         "quoted values are unquoted",
			env:          "XF_DB_USER=\"quoted\"\nXF_DB_PASSWORD='single'\n",
			wantUser:     "quoted",
			wantPassword: "single",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := newRunnerWithEnv(t, tt.env)

			user, password := runner.getDatabaseCredentials()
			if user != tt.wantUser {
				t.Errorf("user = %q, want %q", user, tt.wantUser)
			}

			if password != tt.wantPassword {
				t.Errorf("password = %q, want %q", password, tt.wantPassword)
			}
		})
	}
}

// TestDatabaseCredentialsEnvironmentOverride ensures the process environment
// still wins, matching how docker compose resolves variables.
func TestDatabaseCredentialsEnvironmentOverride(t *testing.T) {
	runner := newRunnerWithEnv(t, "XF_DB_USER=fromfile\nXF_DB_PASSWORD=fromfile\n")

	t.Setenv("XF_DB_USER", "fromenv")
	t.Setenv("XF_DB_PASSWORD", "envsecret")

	user, password := runner.getDatabaseCredentials()
	if user != "fromenv" {
		t.Errorf("user = %q, want the environment to take precedence", user)
	}

	if password != "envsecret" {
		t.Errorf("password = %q, want the environment to take precedence", password)
	}
}
