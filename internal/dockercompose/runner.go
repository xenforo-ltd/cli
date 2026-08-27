// Package dockercompose provides Docker Compose CLI operations for XenForo.
package dockercompose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/xf"
)

var (
	// ErrUnexpectedOutput indicates that an operation produced unexpected output.
	ErrUnexpectedOutput = errors.New("unexpected output")

	// ErrTimeout indicates an operation timed out.
	ErrTimeout = errors.New("operation timed out")

	// ErrEnvNotInitialized indicates the Docker environment is not initialized.
	ErrEnvNotInitialized = errors.New("environment not initialized")
)

// Runner handles Docker Compose operations for a XenForo installation.
type Runner struct {
	xfDir    string
	instance string
	contexts []string
	envPath  string
}

// NewRunner creates a new Docker Compose runner for the given XenForo directory.
func NewRunner(xfDir string) (*Runner, error) {
	xfPath := filepath.Join(xfDir, "src", "XF.php")
	if _, err := os.Stat(xfPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not a XenForo directory (src/XF.php not found): %w", err)
		}

		return nil, fmt.Errorf("cannot access src/XF.php: %w", err)
	}

	composePath := filepath.Join(xfDir, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Join(fmt.Errorf("environment not initialized (compose.yaml not found): %w", ErrEnvNotInitialized), err)
		}

		return nil, fmt.Errorf("cannot access compose.yaml: %w", err)
	}

	envPath := filepath.Join(xfDir, ".env")
	instance := "xf"

	var contexts []string

	if envData, err := os.ReadFile(envPath); err == nil {
		instance = parseEnvValue(string(envData), "XF_INSTANCE")
		if instance == "" {
			instance = "xf"
		}

		contextsStr := parseEnvValue(string(envData), "XF_CONTEXTS")
		if contextsStr != "" {
			contexts = strings.Split(contextsStr, ":")
		}
	}

	return &Runner{
		xfDir:    xfDir,
		instance: instance,
		contexts: contexts,
		envPath:  envPath,
	}, nil
}

// Instance returns the Docker instance name.
func (r *Runner) Instance() string {
	return r.instance
}

// ExecOutput runs a command in a service, streaming its output to stdout.
//
// Output is streamed rather than buffered so that large results, such as a
// database dump, do not have to fit in memory. Secrets belong in env rather
// than cmd: a value passed as a command argument is visible in the container's
// process list.
func (r *Runner) ExecOutput(ctx context.Context, service string, env map[string]string, stdout io.Writer, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "exec", "-T")
	args = r.appendEnvVars(args, env, "-e")
	args = append(args, service)
	args = append(args, cmd...)

	return r.runDockerCommandWithEnvAndIO(ctx, env, nil, stdout, os.Stderr, args...)
}

// ExecInput runs a command in a service, feeding it from stdin.
func (r *Runner) ExecInput(ctx context.Context, service string, env map[string]string, stdin io.Reader, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "exec", "-T")
	args = r.appendEnvVars(args, env, "-e")
	args = append(args, service)
	args = append(args, cmd...)

	return r.runDockerCommandWithEnvAndIO(ctx, env, stdin, os.Stdout, os.Stderr, args...)
}

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

// Destroy stops the environment and removes its volumes.
//
// This is permanent: the database and any other volume data are deleted. It is
// what removing a worktree needs, since otherwise each discarded feature branch
// leaves a full volume set behind.
func (r *Runner) Destroy(ctx context.Context) error {
	args := r.buildComposeArgs()
	args = append(args, "down", "--volumes", "--remove-orphans")

	return r.runDockerCommandWithEnvAndIO(ctx, nil, os.Stdin, os.Stdout, os.Stderr, args...)
}

// ContainerInfo describes a single container as rendered by `xf ps`.
//
// Status and Ports are display strings derived from the Compose record rather
// than wire fields: `docker compose ps --format json` reports State, Health,
// ExitCode and Publishers, not the combined Status/Ports strings.
type ContainerInfo struct {
	Service string
	Name    string
	State   string
	Status  string
	Ports   string
}

// composeContainerRecord mirrors the fields of the official
// `docker compose ps --format json` schema that `xf ps` renders. Only the
// fields the table needs are decoded; unknown fields are ignored.
type composeContainerRecord struct {
	Service    string                 `json:"Service"`
	Name       string                 `json:"Name"`
	State      string                 `json:"State"`
	Health     string                 `json:"Health"`
	ExitCode   int                    `json:"ExitCode"`
	Publishers []composePortPublisher `json:"Publishers"`
}

// composePortPublisher mirrors one entry of the Compose Publishers array.
type composePortPublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// unknownValue stands in for a value Compose does not report, so a table cell
// is visibly unknown rather than blank.
const unknownValue = "—"

// normalize converts a wire record into the display model used by `xf ps`.
func (rec composeContainerRecord) normalize() ContainerInfo {
	return ContainerInfo{
		Service: rec.Service,
		Name:    rec.Name,
		State:   rec.State,
		Status:  rec.statusText(),
		Ports:   rec.portsText(),
	}
}

// statusText renders a container's status from the fields Compose supplies.
// Health is the most specific signal; an exited or dead container reports how
// it ended; anything else has no status beyond its state.
func (rec composeContainerRecord) statusText() string {
	if rec.Health != "" {
		return rec.Health
	}

	if rec.State == "exited" || rec.State == "dead" {
		return fmt.Sprintf("exit %d", rec.ExitCode)
	}

	return unknownValue
}

// portsText renders the container's published ports in Docker's familiar
// "host:published->target/proto" form, joining multiple mappings with commas.
func (rec composeContainerRecord) portsText() string {
	if len(rec.Publishers) == 0 {
		return unknownValue
	}

	mappings := make([]string, 0, len(rec.Publishers))
	for _, publisher := range rec.Publishers {
		mappings = append(mappings, publisher.text())
	}

	return strings.Join(mappings, ", ")
}

// text renders a single port mapping. A mapping with no published port is
// exposed but not forwarded to the host.
func (pub composePortPublisher) text() string {
	target := strconv.Itoa(pub.TargetPort) + "/" + pub.Protocol
	if pub.PublishedPort == 0 {
		return target
	}

	return net.JoinHostPort(pub.URL, strconv.Itoa(pub.PublishedPort)) + "->" + target
}

// PSInfo returns container status parsed from `docker compose ps --format json`.
func (r *Runner) PSInfo(ctx context.Context) ([]ContainerInfo, error) {
	cmd := r.buildDockerCommand(ctx, "ps", "--format", "json")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// stderr carries the useful diagnosis (bad compose file, daemon not
		// running); without it the caller only sees "exit status N".
		if detail := bytes.TrimSpace(stderr.Bytes()); len(detail) > 0 {
			return nil, contextError(ctx, fmt.Errorf("docker command failed: %w: %s", err, detail))
		}

		return nil, contextError(ctx, fmt.Errorf("docker command failed: %w", err))
	}

	return parseContainerInfo(stdout.Bytes())
}

// parseContainerInfo decodes `docker compose ps --format json` output.
//
// Compose has emitted two top-level shapes: a single JSON array in older
// releases, and JSON Lines (one container per line) in newer ones. Both are
// accepted; the records themselves use the official schema, whose status and
// port information lives in Health, ExitCode and Publishers.
func parseContainerInfo(data []byte) ([]ContainerInfo, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []ContainerInfo{}, nil
	}

	var records []composeContainerRecord

	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &records); err != nil {
			return nil, fmt.Errorf("failed to parse container status: %w", err)
		}
	} else {
		decoder := json.NewDecoder(bytes.NewReader(trimmed))

		for {
			var record composeContainerRecord

			err := decoder.Decode(&record)
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return nil, fmt.Errorf("failed to parse container status: %w", err)
			}

			records = append(records, record)
		}
	}

	containers := make([]ContainerInfo, 0, len(records))
	for _, record := range records {
		containers = append(containers, record.normalize())
	}

	return containers, nil
}

// ExecOrRun uses exec for running services and falls back to run for stopped services.
//
// env is passed as container environment variables so secrets stay out of the
// process list. The streams are explicit so interactive commands and progress
// writers behave the same whether the service is running or stopped.
func (r *Runner) ExecOrRun(ctx context.Context, service string, env map[string]string, stdin io.Reader, stdout, stderr io.Writer, cmd ...string) error {
	running, err := r.isServiceRunning(ctx, service)
	if err != nil {
		return err
	}

	if running {
		execArgs := r.buildComposeArgs()
		execArgs = append(execArgs, "exec")
		execArgs = r.appendEnvVars(execArgs, env, "-e")
		execArgs = append(execArgs, service)
		execArgs = append(execArgs, cmd...)

		stderrOutput, err := r.runDockerCommandCaptureStderrWithEnv(ctx, env, stdin, stdout, execArgs...)
		if err != nil && isNotRunningExecError(err, stderrOutput) {
			return r.run(ctx, service, env, stdin, stdout, stderr, cmd...)
		}

		// stderr is captured only to detect the not-running case above. For any
		// other failure it is the command's own error message, so replay it
		// rather than let the caller exit silently.
		if err != nil && stderrOutput != "" {
			_, _ = io.WriteString(stderr, stderrOutput)
		}

		return err
	}

	return r.run(ctx, service, env, stdin, stdout, stderr, cmd...)
}

func (r *Runner) run(ctx context.Context, service string, env map[string]string, stdin io.Reader, stdout, stderr io.Writer, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "run", "--rm")
	args = r.appendEnvVars(args, env, "--env")
	args = append(args, service)
	args = append(args, cmd...)

	return r.runDockerCommandWithEnvAndIO(ctx, env, stdin, stdout, stderr, args...)
}

// Compose runs a docker compose command directly with explicit stdio.
func (r *Runner) Compose(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	composeArgs := r.buildComposeArgs()
	composeArgs = append(composeArgs, args...)

	return r.runDockerCommandWithEnvAndIO(ctx, nil, stdin, stdout, stderr, composeArgs...)
}

// GetURL returns the URL for accessing the XenForo site.
// It detects OrbStack vs standard Docker.
func (r *Runner) GetURL(ctx context.Context) (string, error) {
	isOrbStack := false

	if info, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.OperatingSystem}}").Output(); err == nil {
		if strings.TrimSpace(string(info)) == "OrbStack" {
			isOrbStack = true
		}
	}

	if isOrbStack {
		return fmt.Sprintf("https://%s.xf.local", r.instance), nil
	}

	port, err := r.getServicePort(ctx, "caddy", "80")
	if err != nil {
		return "", err
	}

	return "http://localhost:" + port, nil
}

// WaitForReady waits for the xf container to be ready to accept commands.
func (r *Runner) WaitForReady(ctx context.Context, checkInterval time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for containers to be ready: %w", ctx.Err())
		default:
			cmd := r.buildDockerCommand(ctx, "run", "--rm", "xf", "php", "-v")
			if err := cmd.Run(); err == nil {
				return nil
			}

			time.Sleep(checkInterval)
		}
	}
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
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for database: %w", ctx.Err())
		default:
			args := r.buildComposeArgs()
			args = append(args, "exec", "-T", "xf", "php", "-r", testScript)

			cmd := exec.CommandContext(ctx, "docker", args...)
			cmd.Dir = r.xfDir

			output, err := cmd.Output()
			if err == nil && strings.Contains(string(output), "OK") {
				return nil
			}

			time.Sleep(checkInterval)
		}
	}

	return fmt.Errorf("timed out waiting for database to be ready: %w", ErrTimeout)
}

func escapePHPString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "'", "\\'")
}

// composeProjectLabel is the label Docker Compose stamps on every container and
// volume it creates for a project.
const composeProjectLabel = "com.docker.compose.project"

// ProjectExists reports whether a Compose project already owns containers or
// volumes under the given instance name.
//
// Compose derives container and volume names from the project name, so a second
// environment reusing one would adopt the first's resources. Volumes are checked
// as well as containers because `docker compose down` leaves volumes behind: an
// environment can be stopped with no containers at all and still collide.
//
// A failed Docker command is returned as an error rather than reported as an
// absent project. Treating an unreachable Docker as "free" would let creation
// proceed onto a name that may already be in use.
func ProjectExists(ctx context.Context, instance string) (bool, error) {
	probes := [][]string{
		{"ps", "--all", "--filter", "label=" + composeProjectLabel + "=" + instance, "--format", "{{.ID}}"},
		{"volume", "ls", "--filter", "label=" + composeProjectLabel + "=" + instance, "--format", "{{.Name}}"},
	}

	for _, args := range probes {
		cmd := exec.CommandContext(ctx, "docker", args...)

		out, err := cmd.Output()
		if err != nil {
			return false, contextError(ctx, fmt.Errorf("failed to inspect Docker Compose project %q: %w", instance, err))
		}

		if strings.TrimSpace(string(out)) != "" {
			return true, nil
		}
	}

	return false, nil
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

// runDockerCommandWithEnvAndIO executes a docker command with explicit streams,
// passing env through this process's environment rather than the docker argv, so
// secret values never appear in the host's process list.
func (r *Runner) runDockerCommandWithEnvAndIO(
	ctx context.Context,
	env map[string]string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	args ...string,
) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.xfDir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(), "XF_DIR="+r.xfDir)
	cmd.Env = append(cmd.Env, envPairs(env)...)

	if err := cmd.Run(); err != nil {
		return contextError(ctx, fmt.Errorf("docker command failed: %w", err))
	}

	return nil
}

// runDockerCommandCaptureStderrWithEnv runs a docker command and captures its
// stderr so callers can inspect a failure, forwarding env through this
// process's environment.
func (r *Runner) runDockerCommandCaptureStderrWithEnv(
	ctx context.Context,
	env map[string]string,
	stdin io.Reader,
	stdout io.Writer,
	args ...string,
) (string, error) {
	var stderr bytes.Buffer

	err := r.runDockerCommandWithEnvAndIO(ctx, env, stdin, stdout, &stderr, args...)

	return stderr.String(), err
}

// contextError replaces an execution failure with the context's own error when
// the context has ended.
//
// exec.CommandContext kills the child process on cancellation, which surfaces as
// "signal: killed" and describes the symptom rather than the cause. Substituting
// the context error lets callers tell an interrupt or timeout apart from the
// command genuinely failing.
//
// Returns err unchanged when the context is still live.
func contextError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	return err
}

// buildDockerCommand creates a docker command with the given args.
func (r *Runner) buildDockerCommand(ctx context.Context, extraArgs ...string) *exec.Cmd {
	args := r.buildComposeArgs()
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.xfDir
	cmd.Env = append(os.Environ(), "XF_DIR="+r.xfDir)

	return cmd
}

// buildComposeArgs builds the docker compose command arguments.
func (r *Runner) buildComposeArgs() []string {
	args := []string{"compose", "--project-name", r.instance}

	args = append(args, "--file", filepath.Join(r.xfDir, "compose.yaml"))

	for _, ctx := range r.contexts {
		file := fmt.Sprintf("compose.%s.yaml", ctx)

		filePath := filepath.Join(r.xfDir, file)
		if _, err := os.Stat(filePath); err == nil {
			args = append(args, "--file", filePath)
		}
	}

	overridePath := filepath.Join(r.xfDir, "compose.override.yaml")
	if _, err := os.Stat(overridePath); err == nil {
		args = append(args, "--file", overridePath)
	}

	return args
}

// appendEnvVars appends environment variables to docker arguments, passing the
// nearby name only so docker forwards the value from this process's own
// environment. flagFormat should be either "-e" for exec or "--env" for run.
func (r *Runner) appendEnvVars(args []string, env map[string]string, flagFormat string) []string {
	for _, k := range sortedEnvKeys(env) {
		args = append(args, flagFormat, k)
	}

	return args
}

// sortedEnvKeys returns the keys of env in a stable order, so the arguments a
// command is built with do not vary between runs.
func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// envPairs renders env as "NAME=value" strings for a command's own environment.
func envPairs(env map[string]string) []string {
	pairs := make([]string, 0, len(env))
	for _, k := range sortedEnvKeys(env) {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, env[k]))
	}

	return pairs
}

// getServicePort gets the exposed port for a service.
func (r *Runner) getServicePort(ctx context.Context, service, internalPort string) (string, error) {
	args := r.buildComposeArgs()
	args = append(args, "port", service, internalPort)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.xfDir

	output, err := cmd.Output()
	if err != nil {
		return "", contextError(ctx, fmt.Errorf("failed to get port for %s: %w", service, err))
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ":")
	if len(parts) >= 2 {
		return parts[len(parts)-1], nil
	}

	return "", fmt.Errorf("unexpected port output: %s: %w", output, ErrUnexpectedOutput)
}

func (r *Runner) isServiceRunning(ctx context.Context, service string) (bool, error) {
	args := r.buildComposeArgs()
	args = append(args, "ps", "--status", "running", "--services", service)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.xfDir

	output, err := cmd.Output()
	if err != nil {
		return false, contextError(ctx, fmt.Errorf("failed to check running status for service %s: %w", service, err))
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == service {
			return true, nil
		}
	}

	return false, nil
}

func isNotRunningExecError(err error, stderr string) bool {
	msg := strings.ToLower(err.Error() + " " + stderr)

	return strings.Contains(msg, "is not running") ||
		strings.Contains(msg, "not running") ||
		strings.Contains(msg, "no container found") ||
		strings.Contains(msg, "container") && strings.Contains(msg, "not found")
}

// parseEnvValue extracts a value from .env file content.
func parseEnvValue(content, key string) string {
	prefix := key + "="

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		value := xf.StripQuotes(line[len(prefix):])

		return value
	}

	return ""
}

// CheckDockerRunning checks if Docker is running.
func CheckDockerRunning(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "info")
	if err := cmd.Run(); err != nil {
		return contextError(ctx, fmt.Errorf("docker not running: %w", err))
	}

	return nil
}

// CheckDockerComposeAvailable checks if Docker Compose is available.
func CheckDockerComposeAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		return contextError(ctx, fmt.Errorf("docker compose not available: %w", err))
	}

	return nil
}
