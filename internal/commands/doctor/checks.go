// Package doctor provides health checks for CLI dependencies and configuration.
package doctor

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
)

// checkResult represents the result of a health check.
type checkResult struct {
	name       string
	status     checkStatus
	message    string
	details    string
	suggestion string
}

// checkStatus represents a health check status.
type checkStatus int

const (
	// statusOK indicates a successful check.
	statusOK checkStatus = iota
	// statusWarning indicates a non-critical issue.
	statusWarning
	// statusError indicates a critical issue.
	statusError
	// statusSkipped indicates the check was skipped.
	statusSkipped
)

// doctor performs health checks on the system.
type doctor struct {
	results []*checkResult
}

// newDoctor creates a new doctor instance.
func newDoctor() *doctor {
	return &doctor{}
}

// runAll runs all health checks.
func (d *doctor) runAll(ctx context.Context) []*checkResult {
	d.results = make([]*checkResult, 0)

	store, err := auth.NewStore()
	var token *auth.Token
	var source string
	if err == nil {
		source, token, err = resolveAuthentication(store, auth.RequireAuthFrom)
	}
	d.checkAuthentication(source, token, err)
	d.checkGit(ctx)
	d.checkDocker(ctx)
	d.checkCacheDirectory()
	d.checkDiskSpace(ctx)
	d.checkNetwork(ctx)

	return d.results
}

// resolveAuthentication records the credential source, preflights writable
// persistent stores, and only then loads credentials. Environment-backed stores
// are read-only and are not probed.
func resolveAuthentication(store auth.Store, load func(auth.Store) (*auth.Token, error)) (source string, token *auth.Token, err error) {
	source = store.Source()
	if writable, ok := store.(auth.WritableStore); ok {
		if preflightErr := writable.PrepareLogin(); preflightErr != nil {
			return source, nil, preflightErr
		}
	}
	token, err = load(store)

	return source, token, err
}

// checkAuthentication renders one credential resolution shared by both checks.
func (d *doctor) checkAuthentication(source string, token *auth.Token, err error) {
	storage := &checkResult{name: "Credential Storage", status: statusOK, message: "Credential source: " + source}
	result := &checkResult{name: "Authentication"}
	d.results = append(d.results, storage, result)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrAuthRequired):
			result.status = statusWarning
			result.message = "Not authenticated"
			result.suggestion = "Run " + ui.Command.Render("xf auth login") + " to authenticate"
		case errors.Is(err, auth.ErrConfigMismatch):
			result.status = statusWarning
			result.message = "Authenticated with different configuration"
			result.suggestion = "Run " + ui.Command.Render("xf auth login") + " to re-authenticate"
		default:
			storage.status = statusError
			storage.message = "Unable to use credential storage"
			storage.details = auth.ErrorMessage(err)
			result.status = statusSkipped
			result.message = "Credential storage must be fixed before authentication can be checked"
		}

		return
	}
	if token.External {
		result.status = statusOK
		result.message = "XF_TOKEN is present (validity and expiry not checked)"
		result.suggestion = "Run " + ui.Command.Render("xf auth status") + " for server validation"
		return
	}

	switch {
	case token.IsExpired():
		result.status = statusWarning
		result.message = "Authentication token has expired"
		result.suggestion = "Run " + ui.Command.Render("xf auth login") + " to re-authenticate"
	case token.IsExpiringSoon(10 * time.Minute):
		result.status = statusWarning
		result.message = fmt.Sprintf("Token expires in %s", token.TimeUntilExpiry().Round(time.Minute))
		result.suggestion = "Consider re-authenticating soon with " + ui.Command.Render("xf auth login")
	default:
		result.status = statusOK
		result.message = "Authenticated; credential source: " + source
		result.details = fmt.Sprintf("Expires in %s", token.TimeUntilExpiry().Round(time.Hour))
	}
}

func trimNewline(output []byte) string {
	return strings.TrimSuffix(string(output), "\n")
}

func (d *doctor) checkGit(ctx context.Context) {
	result := &checkResult{
		name: "Git",
	}

	cmd := exec.CommandContext(ctx, "git", "--version")

	output, err := cmd.Output()
	if err != nil {
		result.status = statusError
		result.message = "Git is not installed or not in PATH"
		result.suggestion = "Install Git: https://git-scm.com/downloads"
	} else {
		result.status = statusOK
		result.message = trimNewline(output)
	}

	d.results = append(d.results, result)
}

func (d *doctor) checkDocker(ctx context.Context) {
	result := &checkResult{
		name: "Docker",
	}

	versionCmd := exec.CommandContext(ctx, "docker", "--version")

	versionOutput, err := versionCmd.Output()
	if err != nil {
		result.status = statusError
		result.message = "Docker is not installed or not in PATH"
		result.suggestion = "Install Docker: https://docs.docker.com/get-docker/"
		d.results = append(d.results, result)

		return
	}

	infoCmd := exec.CommandContext(ctx, "docker", "info")
	if err := infoCmd.Run(); err != nil {
		result.status = statusError
		result.message = "Docker daemon is not running"
		result.details = trimNewline(versionOutput)
		result.suggestion = "Start Docker Desktop or the Docker daemon"
		d.results = append(d.results, result)

		return
	}

	result.status = statusOK
	result.message = trimNewline(versionOutput)

	d.results = append(d.results, result)
}

func (d *doctor) checkCacheDirectory() {
	result := &checkResult{
		name: "Cache Directory",
	}

	cfg, err := config.Load()
	if err != nil {
		result.status = statusError
		result.message = "Failed to load configuration"
		result.details = err.Error()
		d.results = append(d.results, result)

		return
	}

	cacheDir := cfg.CachePath

	info, err := os.Stat(cacheDir)
	if errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(cacheDir, 0o750); err != nil {
			result.status = statusError
			result.message = "Cannot create cache directory"
			result.details = fmt.Sprintf("Path: %s\nError: %s", cacheDir, err.Error())
			result.suggestion = "Check permissions for the parent directory"
		} else {
			result.status = statusOK
			result.message = "Cache directory created: " + cacheDir
		}

		d.results = append(d.results, result)

		return
	}

	if err != nil {
		result.status = statusError
		result.message = "Failed to check cache directory"
		result.details = err.Error()
		d.results = append(d.results, result)

		return
	}

	if !info.IsDir() {
		result.status = statusError
		result.message = "Cache path exists but is not a directory"
		result.details = cacheDir
		d.results = append(d.results, result)

		return
	}

	testFile := filepath.Join(cacheDir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		result.status = statusError
		result.message = "Cache directory is not writable"
		result.details = cacheDir
		result.suggestion = "Check directory permissions"
		d.results = append(d.results, result)

		return
	}

	if err := os.Remove(testFile); err != nil {
		result.status = statusWarning
		result.message = fmt.Sprintf("Cache directory writable but cleanup failed: %v", err)
	} else {
		result.status = statusOK
		result.message = "Cache directory: " + cacheDir
	}

	d.results = append(d.results, result)
}

func (d *doctor) checkDiskSpace(ctx context.Context) {
	result := &checkResult{
		name: "Disk Space",
	}

	cfg, err := config.Load()
	if err != nil {
		result.status = statusSkipped
		result.message = "Could not load configuration"
		d.results = append(d.results, result)

		return
	}

	cacheDir := cfg.CachePath

	// Use df command to check disk space (works on macOS and Linux).
	cmd := exec.CommandContext(ctx, "df", "-k", "--", cacheDir)

	output, err := cmd.Output()
	if err != nil {
		result.status = statusSkipped
		result.message = "Could not check disk space"
		d.results = append(d.results, result)

		return
	}

	// Parse df output - second line contains the data.
	// Format: Filesystem 1K-blocks Used Available Use% Mounted
	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	if len(lines) < 2 {
		result.status = statusSkipped
		result.message = "Could not parse disk space info"
		d.results = append(d.results, result)

		return
	}

	var available int64

	_, err = fmt.Sscanf(lines[1], "%s %d %d %d", new(string), new(int64), new(int64), &available)
	if err != nil {
		// Try alternative parsing for different df output formats.
		fields := strings.Fields(lines[1])
		if len(fields) >= 4 {
			if _, err := fmt.Sscanf(fields[3], "%d", &available); err != nil {
				result.status = statusSkipped
				result.message = "Could not parse available disk space"
				d.results = append(d.results, result)

				return
			}
		}
	}

	// available is in 1K blocks, convert to bytes.
	availableBytes := available * 1024

	const minRequired = 1 * 1024 * 1024 * 1024 // 1 GB

	if availableBytes < minRequired {
		result.status = statusWarning
		result.message = fmt.Sprintf("Low disk space: %s available", ui.FormatBytes(availableBytes))
		result.suggestion = "XenForo downloads can be large. Consider freeing up disk space."
	} else {
		result.status = statusOK
		result.message = "Available disk space: " + ui.FormatBytes(availableBytes)
	}

	d.results = append(d.results, result)
}

func (d *doctor) checkNetwork(ctx context.Context) {
	result := &checkResult{
		name: "Network Connectivity",
	}

	targets := []struct {
		name string
		url  string
	}{
		{"GitHub", "https://api.github.com"},
		{"XenForo", "https://xenforo.com"},
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	allOK := true

	var details []string

	for _, target := range targets {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, target.url, nil)
		if err != nil {
			details = append(details, target.name+": failed to create request")
			allOK = false

			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			details = append(details, target.name+": unreachable")
			allOK = false

			continue
		}

		resp.Body.Close()

		details = append(details, target.name+": OK")
	}

	if allOK {
		result.status = statusOK
		result.message = "All endpoints reachable"
	} else {
		result.status = statusWarning
		result.message = "Some endpoints unreachable"
		result.suggestion = "Check your internet connection or firewall settings"
	}

	result.details = strings.Join(details, "\n")

	d.results = append(d.results, result)
}
