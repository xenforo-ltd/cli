// Package doctor provides health checks for CLI dependencies and configuration.
package doctor

import (
	"context"
	"errors"
	"fmt"
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
	Name       string
	Status     checkStatus
	Message    string
	Details    string
	Suggestion string
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
	return &doctor{
		results: make([]*checkResult, 0),
	}
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
	storage := &checkResult{Name: "Credential Storage", Status: statusOK, Message: "Credential source: " + source}
	result := &checkResult{Name: "Authentication"}
	d.results = append(d.results, storage, result)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrAuthRequired):
			result.Status = statusWarning
			result.Message = "Not authenticated"
			result.Suggestion = "Run " + ui.Command.Render("xf auth login") + " to authenticate"
		case errors.Is(err, auth.ErrConfigMismatch):
			result.Status = statusWarning
			result.Message = "Authenticated with different configuration"
			result.Suggestion = "Run " + ui.Command.Render("xf auth login") + " to re-authenticate"
		default:
			storage.Status = statusError
			storage.Message = "Unable to use credential storage"
			storage.Details = auth.ErrorMessage(err)
			result.Status = statusSkipped
			result.Message = "Credential storage must be fixed before authentication can be checked"
		}

		return
	}
	if token.External {
		result.Status = statusOK
		result.Message = "XF_TOKEN is present (validity and expiry not checked)"
		result.Suggestion = "Run " + ui.Command.Render("xf auth status") + " for server validation"
		return
	}

	switch {
	case token.IsExpired():
		result.Status = statusWarning
		result.Message = "Authentication token has expired"
		result.Suggestion = "Run " + ui.Command.Render("xf auth login") + " to re-authenticate"
	case token.IsExpiringSoon(10 * time.Minute):
		result.Status = statusWarning
		result.Message = fmt.Sprintf("Token expires in %s", token.TimeUntilExpiry().Round(time.Minute))
		result.Suggestion = "Consider re-authenticating soon with " + ui.Command.Render("xf auth login")
	default:
		result.Status = statusOK
		result.Message = "Authenticated; credential source: " + source
		result.Details = fmt.Sprintf("Expires in %s", token.TimeUntilExpiry().Round(time.Hour))
	}
}

func (d *doctor) checkGit(ctx context.Context) {
	result := &checkResult{
		Name: "Git",
	}

	cmd := exec.CommandContext(ctx, "git", "--version")

	output, err := cmd.Output()
	if err != nil {
		result.Status = statusError
		result.Message = "Git is not installed or not in PATH"
		result.Suggestion = "Install Git: https://git-scm.com/downloads"
	} else {
		result.Status = statusOK
		result.Message = string(output)[:len(output)-1] // Remove trailing newline
	}

	d.results = append(d.results, result)
}

func (d *doctor) checkDocker(ctx context.Context) {
	result := &checkResult{
		Name: "Docker",
	}

	versionCmd := exec.CommandContext(ctx, "docker", "--version")

	versionOutput, err := versionCmd.Output()
	if err != nil {
		result.Status = statusError
		result.Message = "Docker is not installed or not in PATH"
		result.Suggestion = "Install Docker: https://docs.docker.com/get-docker/"
		d.results = append(d.results, result)

		return
	}

	infoCmd := exec.CommandContext(ctx, "docker", "info")
	if err := infoCmd.Run(); err != nil {
		result.Status = statusError
		result.Message = "Docker daemon is not running"
		result.Details = string(versionOutput)[:len(versionOutput)-1]
		result.Suggestion = "Start Docker Desktop or the Docker daemon"
		d.results = append(d.results, result)

		return
	}

	result.Status = statusOK
	result.Message = string(versionOutput)[:len(versionOutput)-1]

	d.results = append(d.results, result)
}

func (d *doctor) checkCacheDirectory() {
	result := &checkResult{
		Name: "Cache Directory",
	}

	cfg, err := config.Load()
	if err != nil {
		result.Status = statusError
		result.Message = "Failed to load configuration"
		result.Details = err.Error()
		d.results = append(d.results, result)

		return
	}

	cacheDir := cfg.CachePath

	info, err := os.Stat(cacheDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, 0o750); err != nil {
			result.Status = statusError
			result.Message = "Cannot create cache directory"
			result.Details = fmt.Sprintf("Path: %s\nError: %s", cacheDir, err.Error())
			result.Suggestion = "Check permissions for the parent directory"
		} else {
			result.Status = statusOK
			result.Message = "Cache directory created: " + cacheDir
		}

		d.results = append(d.results, result)

		return
	} else if err != nil {
		result.Status = statusError
		result.Message = "Failed to check cache directory"
		result.Details = err.Error()
		d.results = append(d.results, result)

		return
	}

	if !info.IsDir() {
		result.Status = statusError
		result.Message = "Cache path exists but is not a directory"
		result.Details = cacheDir
		d.results = append(d.results, result)

		return
	}

	testFile := filepath.Join(cacheDir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		result.Status = statusError
		result.Message = "Cache directory is not writable"
		result.Details = cacheDir
		result.Suggestion = "Check directory permissions"
	} else {
		if err := os.Remove(testFile); err != nil {
			result.Status = statusWarning
			result.Message = fmt.Sprintf("Cache directory writable but cleanup failed: %v", err)
		} else {
			result.Status = statusOK
			result.Message = "Cache directory: " + cacheDir
		}
	}

	d.results = append(d.results, result)
}

func (d *doctor) checkDiskSpace(ctx context.Context) {
	result := &checkResult{
		Name: "Disk Space",
	}

	cfg, err := config.Load()
	if err != nil {
		result.Status = statusSkipped
		result.Message = "Could not load configuration"
		d.results = append(d.results, result)

		return
	}

	cacheDir := cfg.CachePath

	// Use df command to check disk space (works on macOS and Linux).
	cmd := exec.CommandContext(ctx, "df", "-k", "--", cacheDir)

	output, err := cmd.Output()
	if err != nil {
		result.Status = statusSkipped
		result.Message = "Could not check disk space"
		d.results = append(d.results, result)

		return
	}

	// Parse df output - second line contains the data.
	// Format: Filesystem 1K-blocks Used Available Use% Mounted
	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	if len(lines) < 2 {
		result.Status = statusSkipped
		result.Message = "Could not parse disk space info"
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
				result.Status = statusSkipped
				result.Message = "Could not parse available disk space"
				d.results = append(d.results, result)

				return
			}
		}
	}

	// available is in 1K blocks, convert to bytes.
	availableBytes := available * 1024

	const minRequired = 1 * 1024 * 1024 * 1024 // 1 GB

	if availableBytes < minRequired {
		result.Status = statusWarning
		result.Message = fmt.Sprintf("Low disk space: %s available", ui.FormatBytes(availableBytes))
		result.Suggestion = "XenForo downloads can be large. Consider freeing up disk space."
	} else {
		result.Status = statusOK
		result.Message = "Available disk space: " + ui.FormatBytes(availableBytes)
	}

	d.results = append(d.results, result)
}

func (d *doctor) checkNetwork(ctx context.Context) {
	result := &checkResult{
		Name: "Network Connectivity",
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
		if target.url == "" {
			continue
		}

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
		result.Status = statusOK
		result.Message = "All endpoints reachable"
	} else {
		result.Status = statusWarning
		result.Message = "Some endpoints unreachable"
		result.Suggestion = "Check your internet connection or firewall settings"
	}

	result.Details = strings.Join(details, "\n")

	d.results = append(d.results, result)
}
