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

// NewRunnerWithEnv creates a runner with explicit environment values.
func NewRunnerWithEnv(xfDir, instance string, contexts []string) (*Runner, error) {
	return &Runner{
		xfDir:    xfDir,
		instance: instance,
		contexts: contexts,
		envPath:  filepath.Join(xfDir, ".env"),
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
func (r *Runner) Up(detach bool) error {
	args := r.buildComposeArgs()
	args = append(args, "up")
	if detach {
		args = append(args, "--detach")
	}

	return r.runDockerCommand(args...)
}

// UpWithOutput starts the Docker containers with custom output writers.
func (r *Runner) UpWithOutput(detach bool, stdout, stderr io.Writer) error {
	args := r.buildComposeArgs()
	args = append(args, "up")
	if detach {
		args = append(args, "--detach")
	}

	return r.runDockerCommandWithOutput(stdout, stderr, args...)
}

// Down stops and removes the Docker containers.
func (r *Runner) Down() error {
	args := r.buildComposeArgs()
	args = append(args, "down")
	return r.runDockerCommand(args...)
}

// PS lists running containers.
func (r *Runner) PS() error {
	args := r.buildComposeArgs()
	args = append(args, "ps")
	return r.runDockerCommand(args...)
}

// Logs shows container logs.
func (r *Runner) Logs(follow bool, services ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "logs")
	if follow {
		args = append(args, "--follow")
	}
	args = append(args, services...)
	return r.runDockerCommand(args...)
}

// Exec runs a command in a running container.
func (r *Runner) Exec(service string, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "exec", service)
	args = append(args, cmd...)
	return r.runDockerCommand(args...)
}

// ExecWithEnv runs a command in a running container with additional environment variables.
func (r *Runner) ExecWithEnv(service string, env map[string]string, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "exec")
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "-e", fmt.Sprintf("%s=%s", k, env[k]))
		}
	}
	args = append(args, service)
	args = append(args, cmd...)
	return r.runDockerCommand(args...)
}

// Run runs a one-off command in a new container.
func (r *Runner) Run(service string, rm bool, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "run")
	if rm {
		args = append(args, "--rm")
	}
	args = append(args, service)
	args = append(args, cmd...)
	return r.runDockerCommand(args...)
}

// ExecOrRun uses exec for running services and falls back to run for stopped services.
func (r *Runner) ExecOrRun(service string, rm bool, cmd ...string) error {
	running, err := r.isServiceRunning(service)
	if err != nil {
		return err
	}
	if running {
		execArgs := r.buildComposeArgs()
		execArgs = append(execArgs, "exec", service)
		execArgs = append(execArgs, cmd...)
		stderr, err := r.runDockerCommandCaptureStderr(execArgs...)
		if err != nil && isNotRunningExecError(err, stderr) {
			return r.Run(service, rm, cmd...)
		}
		return err
	}
	return r.Run(service, rm, cmd...)
}

// ExecOrRunWithOutput uses exec for running services and falls back to run for stopped services.
func (r *Runner) ExecOrRunWithOutput(service string, rm bool, stdout, stderr io.Writer, cmd ...string) error {
	running, err := r.isServiceRunning(service)
	if err != nil {
		return err
	}
	if running {
		execArgs := r.buildComposeArgs()
		execArgs = append(execArgs, "exec", service)
		execArgs = append(execArgs, cmd...)
		stderrOutput, err := r.runDockerCommandCaptureStderrWithOutput(stdout, execArgs...)
		if err != nil && isNotRunningExecError(err, stderrOutput) {
			return r.RunWithOutput(service, rm, stdout, stderr, cmd...)
		}
		return err
	}
	return r.RunWithOutput(service, rm, stdout, stderr, cmd...)
}

// ExecOrRunWithEnv uses exec for running services and falls back to run for stopped services.
func (r *Runner) ExecOrRunWithEnv(service string, rm bool, env map[string]string, cmd ...string) error {
	running, err := r.isServiceRunning(service)
	if err != nil {
		return err
	}
	if running {
		execArgs := r.buildComposeArgs()
		execArgs = append(execArgs, "exec")
		if len(env) > 0 {
			keys := make([]string, 0, len(env))
			for k := range env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				execArgs = append(execArgs, "-e", fmt.Sprintf("%s=%s", k, env[k]))
			}
		}
		execArgs = append(execArgs, service)
		execArgs = append(execArgs, cmd...)
		stderr, err := r.runDockerCommandCaptureStderr(execArgs...)
		if err != nil && isNotRunningExecError(err, stderr) {
			return r.RunWithEnv(service, rm, env, cmd...)
		}
		return err
	}
	return r.RunWithEnv(service, rm, env, cmd...)
}

// ExecOrRunWithEnvAndOutput runs a docker-compose exec or run command with output.
func (r *Runner) ExecOrRunWithEnvAndOutput(service string, rm bool, env map[string]string, stdout, stderr io.Writer, cmd ...string) error {
	running, err := r.isServiceRunning(service)
	if err != nil {
		return err
	}
	if running {
		execArgs := r.buildComposeArgs()
		execArgs = append(execArgs, "exec")
		if len(env) > 0 {
			keys := make([]string, 0, len(env))
			for k := range env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				execArgs = append(execArgs, "-e", fmt.Sprintf("%s=%s", k, env[k]))
			}
		}
		execArgs = append(execArgs, service)
		execArgs = append(execArgs, cmd...)
		stderrOutput, err := r.runDockerCommandCaptureStderrWithOutput(stdout, execArgs...)
		if err != nil && isNotRunningExecError(err, stderrOutput) {
			return r.RunWithEnvAndOutput(service, rm, env, stdout, stderr, cmd...)
		}
		return err
	}
	return r.RunWithEnvAndOutput(service, rm, env, stdout, stderr, cmd...)
}

// RunWithEnvAndOutput runs a docker-compose run command with custom output.
func (r *Runner) RunWithEnvAndOutput(service string, rm bool, env map[string]string, stdout, stderr io.Writer, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "run")
	if rm {
		args = append(args, "--rm")
	}
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "--env", fmt.Sprintf("%s=%s", k, env[k]))
		}
	}
	args = append(args, service)
	args = append(args, cmd...)
	return r.runDockerCommandWithOutput(stdout, stderr, args...)
}

// Compose runs a docker compose command directly.
func (r *Runner) Compose(args ...string) error {
	composeArgs := r.buildComposeArgs()
	composeArgs = append(composeArgs, args...)
	return r.runDockerCommand(composeArgs...)
}

// Build builds or rebuilds services.
func (r *Runner) Build(pull bool, services ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "build")
	if pull {
		args = append(args, "--pull")
	}
	args = append(args, services...)
	return r.runDockerCommand(args...)
}

// Pull pulls the latest images.
func (r *Runner) Pull(services ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "pull")
	args = append(args, services...)
	return r.runDockerCommand(args...)
}

// Restart restarts containers.
func (r *Runner) Restart(services ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "restart")
	args = append(args, services...)
	return r.runDockerCommand(args...)
}

// Composer runs composer in the xf container.
func (r *Runner) Composer(args ...string) error {
	return r.ExecOrRun("xf", true, append([]string{"composer"}, args...)...)
}

// PHP runs PHP in the xf container.
func (r *Runner) PHP(args ...string) error {
	return r.ExecOrRun("xf", true, append([]string{"php"}, args...)...)
}

// PHPDebug runs PHP with XDebug enabled.
func (r *Runner) PHPDebug(args ...string) error {
	return r.ExecOrRunWithEnv("xf", true, map[string]string{"XDEBUG_SESSION": "1"}, append([]string{"php"}, args...)...)
}

// XFCommand runs a XenForo CLI command.
func (r *Runner) XFCommand(args ...string) error {
	return r.ExecOrRun("xf", true, append([]string{"php", "cmd.php"}, args...)...)
}

// XFCommandWithOutput runs a XenForo CLI command with custom output writers.
func (r *Runner) XFCommandWithOutput(stdout, stderr io.Writer, args ...string) error {
	return r.ExecOrRunWithOutput("xf", true, stdout, stderr, append([]string{"php", "cmd.php"}, args...)...)
}

// XFCommandDebug runs a XenForo CLI command with XDebug.
func (r *Runner) XFCommandDebug(args ...string) error {
	return r.ExecOrRunWithEnv("xf", true, map[string]string{"XDEBUG_SESSION": "1"}, append([]string{"php", "cmd.php"}, args...)...)
}

// RunWithEnv runs a command with additional environment variables.
func (r *Runner) RunWithEnv(service string, rm bool, env map[string]string, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "run")
	if rm {
		args = append(args, "--rm")
	}
	if len(env) > 0 {
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			args = append(args, "--env", fmt.Sprintf("%s=%s", k, env[k]))
		}
	}
	args = append(args, service)
	args = append(args, cmd...)
	return r.runDockerCommand(args...)
}

// RunWithOutput runs a one-off command in a new container with custom output writers.
func (r *Runner) RunWithOutput(service string, rm bool, stdout, stderr io.Writer, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "run")
	if rm {
		args = append(args, "--rm")
	}
	args = append(args, service)
	args = append(args, cmd...)
	return r.runDockerCommandWithOutput(stdout, stderr, args...)
}

// GetURL returns the URL for accessing the XenForo site.
// It detects OrbStack vs standard Docker.
func (r *Runner) GetURL() (string, error) {
	isOrbStack := false
	if info, err := exec.Command("docker", "info", "--format", "{{.OperatingSystem}}").Output(); err == nil {
		if strings.TrimSpace(string(info)) == "OrbStack" {
			isOrbStack = true
		}
	}

	if isOrbStack {
		return fmt.Sprintf("https://%s.xf.local", r.instance), nil
	}

	port, err := r.getServicePort("caddy", "80")
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http://localhost:%s", port), nil
}

// WaitForReady waits for the xf container to be ready to accept commands.
func (r *Runner) WaitForReady(ctx context.Context, checkInterval time.Duration) error {
	for {
		select {
		case <-ctx.Done():
			return clierrors.New(clierrors.CodeDockerCommandFailed, "timed out waiting for containers to be ready")
		default:
			cmd := r.buildDockerCommand("run", "--rm", "xf", "php", "-v")
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

			cmd := exec.Command("docker", args...)
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
func (r *Runner) RunCapture(args ...string) (string, string, error) {
	allArgs := r.buildComposeArgs()
	allArgs = append(allArgs, args...)

	cmd := exec.Command("docker", allArgs...)
	cmd.Dir = r.xfDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("XF_DIR=%s", r.xfDir))

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
func (r *Runner) runDockerCommand(args ...string) error {
	return r.runDockerCommandWithOutput(os.Stdout, os.Stderr, args...)
}

// runDockerCommandWithOutput executes a docker compose command with custom output.
func (r *Runner) runDockerCommandWithOutput(stdout, stderr io.Writer, args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Dir = r.xfDir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin

	cmd.Env = append(os.Environ(), fmt.Sprintf("XF_DIR=%s", r.xfDir))

	if err := cmd.Run(); err != nil {
		return clierrors.Wrapf(clierrors.CodeDockerCommandFailed, err, "docker command failed")
	}
	return nil
}

func (r *Runner) runDockerCommandCaptureStderr(args ...string) (string, error) {
	var stderr bytes.Buffer
	err := r.runDockerCommandWithOutput(os.Stdout, &stderr, args...)
	return stderr.String(), err
}

func (r *Runner) runDockerCommandCaptureStderrWithOutput(stdout io.Writer, args ...string) (string, error) {
	var stderr bytes.Buffer
	err := r.runDockerCommandWithOutput(stdout, &stderr, args...)
	return stderr.String(), err
}

// buildDockerCommand creates a docker command with the given args.
func (r *Runner) buildDockerCommand(extraArgs ...string) *exec.Cmd {
	args := r.buildComposeArgs()
	args = append(args, extraArgs...)

	cmd := exec.Command("docker", args...)
	cmd.Dir = r.xfDir
	cmd.Env = append(os.Environ(), fmt.Sprintf("XF_DIR=%s", r.xfDir))

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

// getServicePort gets the exposed port for a service.
func (r *Runner) getServicePort(service, internalPort string) (string, error) {
	args := r.buildComposeArgs()
	args = append(args, "port", service, internalPort)

	cmd := exec.Command("docker", args...)
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

func (r *Runner) isServiceRunning(service string) (bool, error) {
	args := r.buildComposeArgs()
	args = append(args, "ps", "--status", "running", "--services", service)

	cmd := exec.Command("docker", args...)
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
		value := strings.TrimSpace(line[len(prefix):])
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		return value
	}
	return ""
}

// AutoDetectRunner creates a runner by auto-detecting the XenForo directory.
func AutoDetectRunner() (*Runner, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to get working directory", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return nil, err
	}

	return NewRunner(xfDir)
}

// CheckDockerRunning checks if Docker is running.
func CheckDockerRunning() error {
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		return clierrors.New(clierrors.CodeDockerNotRunning, "Docker is not running. Start Docker Desktop (or docker daemon) and retry")
	}
	return nil
}

// CheckDockerComposeAvailable checks if Docker Compose is available.
func CheckDockerComposeAvailable() error {
	cmd := exec.Command("docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		return clierrors.New(clierrors.CodeDockerNotRunning, "Docker Compose plugin is not available. Install/upgrade Docker and ensure 'docker compose' works")
	}
	return nil
}

// GenerateInstanceName generates a valid Docker instance name from a directory name.
func GenerateInstanceName(dirName string) string {
	return xf.GenerateInstanceName(dirName)
}
