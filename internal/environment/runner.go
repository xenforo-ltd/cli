package environment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/x/term"
)

var (
	// errUnexpectedOutput indicates that an operation produced unexpected output.
	errUnexpectedOutput = errors.New("unexpected output")

	// errTimeout indicates an operation timed out.
	errTimeout = errors.New("operation timed out")

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
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("not a XenForo directory (src/XF.php not found): %w", err)
		}

		return nil, fmt.Errorf("cannot access src/XF.php: %w", err)
	}

	composePath := filepath.Join(xfDir, "compose.yaml")
	if _, err := os.Stat(composePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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

// Contexts returns the compose contexts declared by XF_CONTEXTS, which select
// the environment's feature compose files (for example "mysql", "postgres" or
// "redis").
func (r *Runner) Contexts() []string {
	return r.contexts
}

// ExecOutput runs a command in a service, streaming its output to stdout.
//
// Output is streamed rather than buffered so that large results, such as a
// database dump, do not have to fit in memory. Secrets belong in env rather
// than cmd: a value passed as a command argument is visible in the container's
// process list.
func (r *Runner) ExecOutput(ctx context.Context, service string, env map[string]string, stdout io.Writer, cmd ...string) error {
	args := r.buildExecArgs(service, env, cmd)

	return r.runDockerCommandWithEnvAndIO(ctx, env, nil, stdout, os.Stderr, args...)
}

// ExecInput runs a command in a service, feeding it from stdin.
func (r *Runner) ExecInput(ctx context.Context, service string, env map[string]string, stdin io.Reader, cmd ...string) error {
	args := r.buildExecArgs(service, env, cmd)

	return r.runDockerCommandWithEnvAndIO(ctx, env, stdin, os.Stdout, os.Stderr, args...)
}

func (r *Runner) buildExecArgs(service string, env map[string]string, cmd []string) []string {
	args := r.buildComposeArgs()
	args = append(args, "exec", "-T")
	args = appendEnvVars(args, env, "-e")
	args = append(args, service)

	return append(args, cmd...)
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

// ExecOrRun uses exec for running services and falls back to run for stopped services.
//
// env is passed as container environment variables so secrets stay out of the
// process list. The streams are explicit so interactive commands and progress
// writers behave the same whether the service is running or stopped. A TTY is
// requested only when stdin is a terminal.
func (r *Runner) ExecOrRun(ctx context.Context, service string, env map[string]string, stdin io.Reader, stdout, stderr io.Writer, cmd ...string) error {
	return r.execOrRunInDir(ctx, service, "", env, stdin, stdout, stderr, cmd...)
}

func (r *Runner) execOrRunInDir(ctx context.Context, service, workDir string, env map[string]string, stdin io.Reader, stdout, stderr io.Writer, cmd ...string) error {
	running, err := r.IsServiceRunning(ctx, service)
	if err != nil {
		return err
	}

	if !running {
		return r.run(ctx, service, workDir, env, stdin, stdout, stderr, cmd...)
	}

	execArgs := r.buildComposeArgs()
	execArgs = append(execArgs, "exec")
	if input, ok := stdin.(*os.File); !ok || !term.IsTerminal(input.Fd()) {
		execArgs = append(execArgs, "-T")
	}

	execArgs = appendEnvVars(execArgs, env, "-e")
	execArgs = appendWorkDir(execArgs, workDir)
	execArgs = append(execArgs, service)
	execArgs = append(execArgs, cmd...)

	var capturedStderr bytes.Buffer
	err = r.runDockerCommandWithEnvAndIO(ctx, env, stdin, stdout, &capturedStderr, execArgs...)
	stderrOutput := capturedStderr.String()
	if err != nil && isNotRunningExecError(err, stderrOutput) {
		return r.run(ctx, service, workDir, env, stdin, stdout, stderr, cmd...)
	}

	// stderr is captured only to detect the not-running case above. For any
	// other result, forward it: successful commands also write diagnostics
	// and progress to stderr.
	if stderrOutput != "" {
		if _, writeErr := io.WriteString(stderr, stderrOutput); writeErr != nil && err == nil {
			return writeErr
		}
	}

	return err
}

func (r *Runner) run(ctx context.Context, service, workDir string, env map[string]string, stdin io.Reader, stdout, stderr io.Writer, cmd ...string) error {
	args := r.buildComposeArgs()
	args = append(args, "run", "--rm")
	args = appendEnvVars(args, env, "--env")
	args = appendWorkDir(args, workDir)
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

// composeCommand builds a docker compose command with the given args.
func (r *Runner) composeCommand(ctx context.Context, extraArgs ...string) *exec.Cmd {
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

	for _, name := range r.contexts {
		filePath := filepath.Join(r.xfDir, fmt.Sprintf("compose.%s.yaml", name))
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
func appendEnvVars(args []string, env map[string]string, flagFormat string) []string {
	for _, k := range sortedEnvKeys(env) {
		args = append(args, flagFormat, k)
	}

	return args
}

func appendWorkDir(args []string, workDir string) []string {
	if workDir == "" {
		return args
	}

	return append(args, "--workdir", workDir)
}

// sortedEnvKeys returns the keys of env in a stable order, so the arguments a
// command is built with do not vary between runs.
func sortedEnvKeys(env map[string]string) []string {
	return slices.Sorted(maps.Keys(env))
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

	return "", fmt.Errorf("unexpected port output: %s: %w", output, errUnexpectedOutput)
}

// IsServiceRunning reports whether the given compose service has a running
// container.
func (r *Runner) IsServiceRunning(ctx context.Context, service string) (bool, error) {
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

// HasRunningServices reports whether the compose project has any running
// containers.
func (r *Runner) HasRunningServices(ctx context.Context) (bool, error) {
	containers, err := r.PSInfo(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check running containers: %w", err)
	}

	for _, container := range containers {
		if container.State == "running" {
			return true, nil
		}
	}

	return false, nil
}

func isNotRunningExecError(err error, stderr string) bool {
	msg := strings.ToLower(err.Error() + " " + stderr)

	return strings.Contains(msg, "not running") ||
		strings.Contains(msg, "no container found") ||
		(strings.Contains(msg, "container") && strings.Contains(msg, "not found"))
}

// parseEnvValue extracts a value from .env file content.
func parseEnvValue(content, key string) string {
	prefix := key + "="

	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if value, ok := strings.CutPrefix(line, prefix); ok {
			return stripQuotes(value)
		}
	}

	return ""
}
