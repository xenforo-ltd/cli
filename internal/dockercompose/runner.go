// Package dockercompose provides Docker Compose CLI operations for XenForo.
package dockercompose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/clierrors"
	"github.com/xenforo-ltd/cli/internal/xf"
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
	if _, err := os.Stat(xfPath); os.IsNotExist(err) {
		return nil, clierrors.New(clierrors.CodeInvalidInput, "not a XenForo directory (src/XF.php not found)")
	}

	composePath := filepath.Join(xfDir, "compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		return nil, clierrors.New(clierrors.CodeDockerEnvNotInitialized, "Docker environment not initialized (compose.yaml not found)")
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

// XFDir returns the XenForo directory path.
func (r *Runner) XFDir() string {
	return r.xfDir
}

// Instance returns the Docker instance name.
func (r *Runner) Instance() string {
	return r.instance
}

// Contexts returns the configured contexts.
func (r *Runner) Contexts() []string {
	return r.contexts
}

// Up starts the Docker containers.
func (r *Runner) Up(ctx context.Context, detach bool) error {
	args := r.buildComposeArgs()

	args = append(args, "up")
	if detach {
		args = append(args, "--detach")
	}

	return r.runDockerCommand(ctx, args...)
}

// UpWithOutput starts the Docker containers with custom output writers.
func (r *Runner) UpWithOutput(ctx context.Context, detach bool, stdout, stderr io.Writer) error {
	args := r.buildComposeArgs()

	args = append(args, "up")
	if detach {
		args = append(args, "--detach")
	}

	return r.runDockerCommandWithOutput(ctx, stdout, stderr, args...)
}

// Down stops and removes the Docker containers.
func (r *Runner) Down(ctx context.Context) error {
	args := r.buildComposeArgs()
	args = append(args, "down")

	return r.runDockerCommand(ctx, args...)
}

// PS lists running containers.
func (r *Runner) PS(ctx context.Context) error {
	args := r.buildComposeArgs()
	args = append(args, "ps")

	return r.runDockerCommand(ctx, args...)
}

// Logs shows container logs.
func (r *Runner) Logs(ctx context.Context, follow bool, services ...string) error {
	args := r.buildComposeArgs()

	args = append(args, "logs")
	if follow {
		args = append(args, "--follow")
	}

	args = append(args, services...)

	return r.runDockerCommand(ctx, args...)
}

// Exec runs a command in a running container.
func (r *Runner) Exec(ctx context.Context, service string, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "exec", service)
	args = append(args, cmd...)

	return r.runDockerCommand(ctx, args...)
}

// ExecWithEnv runs a command in a running container with additional environment variables.
func (r *Runner) ExecWithEnv(ctx context.Context, service string, env map[string]string, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "exec")
	args = r.appendEnvVars(args, env, "-e")
	args = append(args, service)
	args = append(args, cmd...)

	return r.runDockerCommand(ctx, args...)
}

// Run runs a one-off command in a new container.
func (r *Runner) Run(ctx context.Context, service string, rm bool, cmd ...string) error {
	args := r.buildComposeArgs()

	args = append(args, "run")
	if rm {
		args = append(args, "--rm")
	}

	args = append(args, service)
	args = append(args, cmd...)

	return r.runDockerCommand(ctx, args...)
}

// ExecOrRun uses exec for running services and falls back to run for stopped services.
func (r *Runner) ExecOrRun(ctx context.Context, service string, rm bool, cmd ...string) error {
	running, err := r.isServiceRunning(ctx, service)
	if err != nil {
		return err
	}

	if running {
		execArgs := r.buildComposeArgs()
		execArgs = append(execArgs, "exec", service)
		execArgs = append(execArgs, cmd...)

		stderr, err := r.runDockerCommandCaptureStderr(ctx, execArgs...)
		if err != nil && isNotRunningExecError(err, stderr) {
			return r.Run(ctx, service, rm, cmd...)
		}

		return err
	}

	return r.Run(ctx, service, rm, cmd...)
}

// ExecOrRunWithOutput uses exec for running services and falls back to run for stopped services.
func (r *Runner) ExecOrRunWithOutput(ctx context.Context, service string, rm bool, stdout, stderr io.Writer, cmd ...string) error {
	running, err := r.isServiceRunning(ctx, service)
	if err != nil {
		return err
	}

	if running {
		execArgs := r.buildComposeArgs()
		execArgs = append(execArgs, "exec", service)
		execArgs = append(execArgs, cmd...)

		stderrOutput, err := r.runDockerCommandCaptureStderrWithOutput(ctx, stdout, execArgs...)
		if err != nil && isNotRunningExecError(err, stderrOutput) {
			return r.RunWithOutput(ctx, service, rm, stdout, stderr, cmd...)
		}

		return err
	}

	return r.RunWithOutput(ctx, service, rm, stdout, stderr, cmd...)
}

// ExecOrRunWithEnv uses exec for running services and falls back to run for stopped services.
func (r *Runner) ExecOrRunWithEnv(ctx context.Context, service string, rm bool, env map[string]string, cmd ...string) error {
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

		stderr, err := r.runDockerCommandCaptureStderr(ctx, execArgs...)
		if err != nil && isNotRunningExecError(err, stderr) {
			return r.RunWithEnv(ctx, service, rm, env, cmd...)
		}

		return err
	}

	return r.RunWithEnv(ctx, service, rm, env, cmd...)
}

// ExecOrRunWithEnvAndOutput runs a docker-compose exec or run command with output.
func (r *Runner) ExecOrRunWithEnvAndOutput(ctx context.Context, service string, rm bool, env map[string]string, stdout, stderr io.Writer, cmd ...string) error {
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

		stderrOutput, err := r.runDockerCommandCaptureStderrWithOutput(ctx, stdout, execArgs...)
		if err != nil && isNotRunningExecError(err, stderrOutput) {
			return r.RunWithEnvAndOutput(ctx, service, rm, env, stdout, stderr, cmd...)
		}

		return err
	}

	return r.RunWithEnvAndOutput(ctx, service, rm, env, stdout, stderr, cmd...)
}

// RunWithEnvAndOutput runs a docker-compose run command with custom output.
func (r *Runner) RunWithEnvAndOutput(ctx context.Context, service string, rm bool, env map[string]string, stdout, stderr io.Writer, cmd ...string) error {
	args := r.buildComposeArgs()

	args = append(args, "run")
	if rm {
		args = append(args, "--rm")
	}

	args = r.appendEnvVars(args, env, "--env")
	args = append(args, service)
	args = append(args, cmd...)

	return r.runDockerCommandWithOutput(ctx, stdout, stderr, args...)
}

// Compose runs a docker compose command directly.
func (r *Runner) Compose(ctx context.Context, args ...string) error {
	composeArgs := r.buildComposeArgs()
	composeArgs = append(composeArgs, args...)

	return r.runDockerCommand(ctx, composeArgs...)
}

// Build builds or rebuilds services.
func (r *Runner) Build(ctx context.Context, pull bool, services ...string) error {
	args := r.buildComposeArgs()

	args = append(args, "build")
	if pull {
		args = append(args, "--pull")
	}

	args = append(args, services...)

	return r.runDockerCommand(ctx, args...)
}

// Pull pulls the latest images.
func (r *Runner) Pull(ctx context.Context, services ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "pull")
	args = append(args, services...)

	return r.runDockerCommand(ctx, args...)
}

// Restart restarts containers.
func (r *Runner) Restart(ctx context.Context, services ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "restart")
	args = append(args, services...)

	return r.runDockerCommand(ctx, args...)
}

// Composer runs composer in the xf container.
func (r *Runner) Composer(ctx context.Context, args ...string) error {
	return r.ExecOrRun(ctx, "xf", true, append([]string{"composer"}, args...)...)
}

// PHP runs PHP in the xf container.
func (r *Runner) PHP(ctx context.Context, args ...string) error {
	return r.ExecOrRun(ctx, "xf", true, append([]string{"php"}, args...)...)
}

// PHPDebug runs PHP with XDebug enabled.
func (r *Runner) PHPDebug(ctx context.Context, args ...string) error {
	return r.ExecOrRunWithEnv(ctx, "xf", true, map[string]string{"XDEBUG_SESSION": "1"}, append([]string{"php"}, args...)...)
}

// XFCommand runs a XenForo CLI command.
func (r *Runner) XFCommand(ctx context.Context, args ...string) error {
	return r.ExecOrRun(ctx, "xf", true, append([]string{"php", "cmd.php"}, args...)...)
}

// XFCommandWithOutput runs a XenForo CLI command with custom output writers.
func (r *Runner) XFCommandWithOutput(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	return r.ExecOrRunWithOutput(ctx, "xf", true, stdout, stderr, append([]string{"php", "cmd.php"}, args...)...)
}

// XFCommandDebug runs a XenForo CLI command with XDebug.
func (r *Runner) XFCommandDebug(ctx context.Context, args ...string) error {
	return r.ExecOrRunWithEnv(ctx, "xf", true, map[string]string{"XDEBUG_SESSION": "1"}, append([]string{"php", "cmd.php"}, args...)...)
}

// RunWithEnv runs a command with additional environment variables.
func (r *Runner) RunWithEnv(ctx context.Context, service string, rm bool, env map[string]string, cmd ...string) error {
	args := r.buildComposeArgs()

	args = append(args, "run")
	if rm {
		args = append(args, "--rm")
	}

	args = r.appendEnvVars(args, env, "--env")
	args = append(args, service)
	args = append(args, cmd...)

	return r.runDockerCommand(ctx, args...)
}

// RunWithOutput runs a one-off command in a new container with custom output writers.
func (r *Runner) RunWithOutput(ctx context.Context, service string, rm bool, stdout, stderr io.Writer, cmd ...string) error {
	args := r.buildComposeArgs()

	args = append(args, "run")
	if rm {
		args = append(args, "--rm")
	}

	args = append(args, service)
	args = append(args, cmd...)

	return r.runDockerCommandWithOutput(ctx, stdout, stderr, args...)
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
			return clierrors.New(clierrors.CodeDockerCommandFailed, "timed out waiting for containers to be ready")
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
	user, password := r.getDatabaseCredentials()
	testScript := fmt.Sprintf(
		"try { new PDO('mysql:host=mysql', '%s', '%s'); echo 'OK'; } catch (Exception $e) { exit(1); }",
		escapePHPString(user),
		escapePHPString(password),
	)

	maxAttempts := 30
	for range maxAttempts {
		select {
		case <-ctx.Done():
			return clierrors.New(clierrors.CodeDockerCommandFailed, "timed out waiting for database")
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

	return clierrors.New(clierrors.CodeDockerCommandFailed, "timed out waiting for database to be ready")
}

func escapePHPString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "'", "\\'")
}

// IsEnvironmentInitialized checks if the Docker environment is set up.
func (r *Runner) IsEnvironmentInitialized() bool {
	composePath := filepath.Join(r.xfDir, "compose.yaml")
	_, err := os.Stat(composePath)

	return err == nil
}

// RunCapture runs a docker compose command and captures output.
func (r *Runner) RunCapture(ctx context.Context, args ...string) (string, string, error) {
	allArgs := r.buildComposeArgs()
	allArgs = append(allArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", allArgs...)
	cmd.Dir = r.xfDir
	cmd.Env = append(os.Environ(), "XF_DIR="+r.xfDir)

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if err != nil {
		err = clierrors.Wrapf(clierrors.CodeDockerCommandFailed, err, "docker command failed")
	}

	return stdout, stderr, err
}

func (r *Runner) getDatabaseCredentials() (string, string) {
	user := "xf"
	password := "password"

	if envData, err := os.ReadFile(r.envPath); err == nil {
		if value := parseEnvValue(string(envData), "MYSQL_USER"); value != "" {
			user = value
		}

		if value := parseEnvValue(string(envData), "MYSQL_PASSWORD"); value != "" {
			password = value
		}
	}

	if value := os.Getenv("MYSQL_USER"); value != "" {
		user = value
	}

	if value := os.Getenv("MYSQL_PASSWORD"); value != "" {
		password = value
	}

	return user, password
}

// runDockerCommand executes a docker compose command.
func (r *Runner) runDockerCommand(ctx context.Context, args ...string) error {
	return r.runDockerCommandWithOutput(ctx, os.Stdout, os.Stderr, args...)
}

// runDockerCommandWithOutput executes a docker compose command with custom output.
func (r *Runner) runDockerCommandWithOutput(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.xfDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin

	cmd.Env = append(os.Environ(), "XF_DIR="+r.xfDir)

	if err := cmd.Run(); err != nil {
		return clierrors.Wrapf(clierrors.CodeDockerCommandFailed, err, "docker command failed")
	}

	return nil
}

func (r *Runner) runDockerCommandCaptureStderr(ctx context.Context, args ...string) (string, error) {
	var stderr bytes.Buffer

	err := r.runDockerCommandWithOutput(ctx, os.Stdout, &stderr, args...)

	return stderr.String(), err
}

func (r *Runner) runDockerCommandCaptureStderrWithOutput(ctx context.Context, stdout io.Writer, args ...string) (string, error) {
	var stderr bytes.Buffer

	err := r.runDockerCommandWithOutput(ctx, stdout, &stderr, args...)

	return stderr.String(), err
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

// appendEnvVars appends environment variables to docker arguments.
// flagFormat should be either "-e" for exec or "--env" for run.
func (r *Runner) appendEnvVars(args []string, env map[string]string, flagFormat string) []string {
	if len(env) == 0 {
		return args
	}

	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		args = append(args, flagFormat, fmt.Sprintf("%s=%s", k, env[k]))
	}

	return args
}

// getServicePort gets the exposed port for a service.
func (r *Runner) getServicePort(ctx context.Context, service, internalPort string) (string, error) {
	args := r.buildComposeArgs()
	args = append(args, "port", service, internalPort)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.xfDir

	output, err := cmd.Output()
	if err != nil {
		return "", clierrors.Wrapf(clierrors.CodeDockerCommandFailed, err, "failed to get port for %s", service)
	}

	parts := strings.Split(strings.TrimSpace(string(output)), ":")
	if len(parts) >= 2 {
		return parts[len(parts)-1], nil
	}

	return "", clierrors.Newf(clierrors.CodeDockerCommandFailed, "unexpected port output: %s", output)
}

func (r *Runner) isServiceRunning(ctx context.Context, service string) (bool, error) {
	args := r.buildComposeArgs()
	args = append(args, "ps", "--status", "running", "--services", service)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = r.xfDir

	output, err := cmd.Output()
	if err != nil {
		return false, clierrors.Wrapf(clierrors.CodeDockerCommandFailed, err, "failed to check running status for service %s", service)
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
		return clierrors.New(clierrors.CodeDockerNotRunning, "Docker is not running. Start Docker Desktop (or docker daemon) and retry")
	}

	return nil
}

// CheckDockerComposeAvailable checks if Docker Compose is available.
func CheckDockerComposeAvailable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		return clierrors.New(clierrors.CodeDockerNotRunning, "Docker Compose plugin is not available. Install/upgrade Docker and ensure 'docker compose' works")
	}

	return nil
}
