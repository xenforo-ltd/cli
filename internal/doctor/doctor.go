// Package doctor provides health checks for CLI dependencies and configuration.
package doctor

import (
	"context"
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

// CheckResult represents the result of a health check.
type CheckResult struct {
	Name       string
	Status     CheckStatus
	Message    string
	Details    string
	Suggestion string
}

// CheckStatus represents a health check status.
type CheckStatus int

const (
	// StatusOK indicates a successful check.
	StatusOK CheckStatus = iota
	// StatusWarning indicates a non-critical issue.
	StatusWarning
	// StatusError indicates a critical issue.
	StatusError
	// StatusSkipped indicates the check was skipped.
	StatusSkipped
)

func (s CheckStatus) String() string {
	switch s {
	case StatusOK:
		return "OK"
	case StatusWarning:
		return "WARNING"
	case StatusError:
		return "ERROR"
	case StatusSkipped:
		return "SKIPPED"
	default:
		return "UNKNOWN"
	}
}

// Symbol returns the symbol representation of the status.
func (s CheckStatus) Symbol() string {
	switch s {
	case StatusOK:
		return "+"
	case StatusWarning:
		return "!"
	case StatusError:
		return "x"
	case StatusSkipped:
		return "-"
	default:
		return "?"
	}
}

// Doctor performs health checks on the system.
type Doctor struct {
	results []*CheckResult
}

// NewDoctor creates a new Doctor instance.
func NewDoctor() *Doctor {
	return &Doctor{
		results: make([]*CheckResult, 0),
	}
}

// RunAll runs all health checks.
func (d *Doctor) RunAll(ctx context.Context) []*CheckResult {
	d.results = make([]*CheckResult, 0)

	d.checkKeychain()
	d.checkAuth()
	d.checkGit(ctx)
	d.checkDocker(ctx)
	d.checkCacheDirectory()
	d.checkDiskSpace(ctx)
	d.checkNetwork(ctx)

	return d.results
}

// Results returns the check results.
func (d *Doctor) Results() []*CheckResult {
	return d.results
}

// HasErrors returns whether any checks failed.
func (d *Doctor) HasErrors() bool {
	for _, r := range d.results {
		if r.Status == StatusError {
			return true
		}
	}

	return false
}

// HasWarnings returns whether any checks reported warnings.
func (d *Doctor) HasWarnings() bool {
	for _, r := range d.results {
		if r.Status == StatusWarning {
			return true
		}
	}

	return false
}

func (d *Doctor) checkKeychain() {
	result := &CheckResult{
		Name: "System Keychain",
	}

	kc := auth.NewKeychain()
	if kc.IsAvailable() {
		result.Status = StatusOK
		result.Message = "Keychain is accessible"
	} else {
		result.Status = StatusError
		result.Message = "Keychain is not accessible"
		result.Suggestion = "Ensure your system keychain service is running. On Linux, this may require gnome-keyring or similar."
	}

	d.results = append(d.results, result)
}

func (d *Doctor) checkAuth() {
	result := &CheckResult{
		Name: "Authentication",
	}

	kc := auth.NewKeychain()

	token, err := kc.LoadToken()
	if err != nil {
		result.Status = StatusWarning
		result.Message = "Not authenticated"
		result.Suggestion = "Run 'xf auth login' to authenticate"
		d.results = append(d.results, result)

		return
	}

	cfg, err := config.Load()
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to load configuration"
		result.Details = err.Error()
		d.results = append(d.results, result)

		return
	}

	if token.BaseURL != cfg.OAuth.BaseURL {
		result.Status = StatusWarning
		result.Message = "Authenticated with different configuration"
		result.Suggestion = "Run 'xf auth login' to re-authenticate"
		d.results = append(d.results, result)

		return
	}

	switch {
	case token.IsExpired():
		result.Status = StatusWarning
		result.Message = "Authentication token has expired"
		result.Suggestion = "Run 'xf auth login' to re-authenticate"
	case token.IsExpiringSoon(10 * time.Minute):
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Token expires in %s", token.TimeUntilExpiry().Round(time.Minute))
		result.Suggestion = "Consider re-authenticating soon with 'xf auth login'"
	default:
		result.Status = StatusOK
		result.Message = "Authenticated"
		result.Details = fmt.Sprintf("Expires in %s", token.TimeUntilExpiry().Round(time.Hour))
	}

	d.results = append(d.results, result)
}

func (d *Doctor) checkGit(ctx context.Context) {
	result := &CheckResult{
		Name: "Git",
	}

	cmd := exec.CommandContext(ctx, "git", "--version")

	output, err := cmd.Output()
	if err != nil {
		result.Status = StatusError
		result.Message = "Git is not installed or not in PATH"
		result.Suggestion = "Install Git: https://git-scm.com/downloads"
	} else {
		result.Status = StatusOK
		result.Message = string(output)[:len(output)-1] // Remove trailing newline
	}

	d.results = append(d.results, result)
}

func (d *Doctor) checkDocker(ctx context.Context) {
	result := &CheckResult{
		Name: "Docker",
	}

	versionCmd := exec.CommandContext(ctx, "docker", "--version")

	versionOutput, err := versionCmd.Output()
	if err != nil {
		result.Status = StatusError
		result.Message = "Docker is not installed or not in PATH"
		result.Suggestion = "Install Docker: https://docs.docker.com/get-docker/"
		d.results = append(d.results, result)

		return
	}

	infoCmd := exec.CommandContext(ctx, "docker", "info")
	if err := infoCmd.Run(); err != nil {
		result.Status = StatusError
		result.Message = "Docker daemon is not running"
		result.Details = string(versionOutput)[:len(versionOutput)-1]
		result.Suggestion = "Start Docker Desktop or the Docker daemon"
		d.results = append(d.results, result)

		return
	}

	result.Status = StatusOK
	result.Message = string(versionOutput)[:len(versionOutput)-1]

	d.results = append(d.results, result)
}

func (d *Doctor) checkCacheDirectory() {
	result := &CheckResult{
		Name: "Cache Directory",
	}

	cfg, err := config.Load()
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to load configuration"
		result.Details = err.Error()
		d.results = append(d.results, result)

		return
	}

	cacheDir := cfg.CachePath

	info, err := os.Stat(cacheDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, 0o750); err != nil {
			result.Status = StatusError
			result.Message = "Cannot create cache directory"
			result.Details = fmt.Sprintf("Path: %s\nError: %s", cacheDir, err.Error())
			result.Suggestion = "Check permissions for the parent directory"
		} else {
			result.Status = StatusOK
			result.Message = "Cache directory created: " + cacheDir
		}

		d.results = append(d.results, result)

		return
	} else if err != nil {
		result.Status = StatusError
		result.Message = "Failed to check cache directory"
		result.Details = err.Error()
		d.results = append(d.results, result)

		return
	}

	if !info.IsDir() {
		result.Status = StatusError
		result.Message = "Cache path exists but is not a directory"
		result.Details = cacheDir
		d.results = append(d.results, result)

		return
	}

	testFile := filepath.Join(cacheDir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0o600); err != nil {
		result.Status = StatusError
		result.Message = "Cache directory is not writable"
		result.Details = cacheDir
		result.Suggestion = "Check directory permissions"
	} else {
		if err := os.Remove(testFile); err != nil {
			result.Status = StatusWarning
			result.Message = fmt.Sprintf("Cache directory writable but cleanup failed: %v", err)
		} else {
			result.Status = StatusOK
			result.Message = "Cache directory: " + cacheDir
		}
	}

	d.results = append(d.results, result)
}

func (d *Doctor) checkDiskSpace(ctx context.Context) {
	result := &CheckResult{
		Name: "Disk Space",
	}

	cfg, err := config.Load()
	if err != nil {
		result.Status = StatusSkipped
		result.Message = "Could not load configuration"
		d.results = append(d.results, result)

		return
	}

	cacheDir := cfg.CachePath

	// Use df command to check disk space (works on macOS and Linux).
	cmd := exec.CommandContext(ctx, "df", "-k", cacheDir)

	output, err := cmd.Output()
	if err != nil {
		result.Status = StatusSkipped
		result.Message = "Could not check disk space"
		d.results = append(d.results, result)

		return
	}

	// Parse df output - second line contains the data.
	// Format: Filesystem 1K-blocks Used Available Use% Mounted
	lines := strings.Split(strings.TrimRight(string(output), "\n"), "\n")
	if len(lines) < 2 {
		result.Status = StatusSkipped
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
				result.Status = StatusSkipped
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
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Low disk space: %s available", ui.FormatBytes(availableBytes))
		result.Suggestion = "XenForo downloads can be large. Consider freeing up disk space."
	} else {
		result.Status = StatusOK
		result.Message = "Available disk space: " + ui.FormatBytes(availableBytes)
	}

	d.results = append(d.results, result)
}

func (d *Doctor) checkNetwork(ctx context.Context) {
	result := &CheckResult{
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
		result.Status = StatusOK
		result.Message = "All endpoints reachable"
	} else {
		result.Status = StatusWarning
		result.Message = "Some endpoints unreachable"
		result.Suggestion = "Check your internet connection or firewall settings"
	}

	result.Details = strings.Join(details, "\n")

	d.results = append(d.results, result)
}
