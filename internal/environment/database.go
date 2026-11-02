package environment

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DatabaseCredentials returns the configured database user and password.
//
// The names must match what the compose files read, since those are the keys
// that end up in .env: XF_DB_USER and XF_DB_PASSWORD (compose.mysql.yaml,
// compose.postgres.yaml). The defaults mirror the fallbacks declared there.
//
// Resolution order matches docker compose: process environment, then .env, then
// the built-in default.
func (r *Runner) DatabaseCredentials() (string, string) {
	return r.resolveEnvValue("XF_DB_USER", "xf"),
		r.resolveEnvValue("XF_DB_PASSWORD", "password")
}

// DatabaseName returns the configured database name.
func (r *Runner) DatabaseName() string {
	return r.resolveEnvValue("XF_DB_DATABASE", "xf")
}

// WaitForDatabase waits for the database to be ready.
func (r *Runner) WaitForDatabase(ctx context.Context, checkInterval time.Duration) error {
	user, password := r.DatabaseCredentials()
	testScript := fmt.Sprintf(
		"try { new PDO('mysql:host=mysql', '%s', '%s'); echo 'OK'; } catch (Exception $e) { exit(1); }",
		escapePHPString(user),
		escapePHPString(password),
	)

	maxAttempts := 30
	for range maxAttempts {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("timed out waiting for database: %w", err)
		}

		args := r.buildComposeArgs()
		args = append(args, "exec", "-T", "xf", "php", "-r", testScript)

		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Dir = r.xfDir

		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), "OK") {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for database: %w", ctx.Err())
		case <-time.After(checkInterval):
		}
	}

	return fmt.Errorf("timed out waiting for database to be ready: %w", errTimeout)
}

func escapePHPString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "'", "\\'")
}

// resolveEnvValue returns the value for key from the process environment, then
// the project's .env file, falling back to def.
func (r *Runner) resolveEnvValue(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	if envData, err := os.ReadFile(r.envPath); err == nil {
		if value := parseEnvValue(string(envData), key); value != "" {
			return value
		}
	}

	return def
}
