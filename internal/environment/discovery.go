package environment

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// IsOrbStack reports whether the Docker engine is OrbStack, which exposes
// container services to the host at <service>.<project>.orb.local without
// publishing ports.
func IsOrbStack(ctx context.Context) (bool, error) {
	info, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.OperatingSystem}}").Output()
	if err != nil {
		return false, contextError(ctx, fmt.Errorf("docker info failed: %w", err))
	}

	return strings.TrimSpace(string(info)) == "OrbStack", nil
}

// GetURL returns the URL for accessing the XenForo site.
// It detects OrbStack vs standard Docker.
func (r *Runner) GetURL(ctx context.Context) (string, error) {
	isOrbStack, err := IsOrbStack(ctx)
	if err != nil {
		return "", err
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
