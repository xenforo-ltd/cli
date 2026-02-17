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

	"xf/internal/auth"
	"xf/internal/config"
)

// CheckResult represents the result of a health check.
type CheckResult struct {
	Name       string
	Status     CheckStatus
	Message    string
	Details    string
	Suggestion string
}

// CheckStatus represents the status of a health check.
type CheckStatus int

const (
	StatusOK CheckStatus = iota
	StatusWarning
	StatusError
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

type Doctor struct {
	results []*CheckResult
}

func NewDoctor() *Doctor {
	return &Doctor{
		results: make([]*CheckResult, 0),
	}
}

func (d *Doctor) RunAll(ctx context.Context) []*CheckResult {
	d.results = make([]*CheckResult, 0)

	d.checkKeychain()
	d.checkAuth()
	d.checkGit()
	d.checkDocker()
	d.checkCacheDirectory()
	d.checkDiskSpace()
	d.checkNetwork(ctx)

	return d.results
}

func (d *Doctor) Results() []*CheckResult {
	return d.results
}

func (d *Doctor) HasErrors() bool {
	for _, r := range d.results {
		if r.Status == StatusError {
			return true
		}
	}
	return false
}

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

	if token.Environment != string(config.GetEffectiveEnvironment()) || token.BaseURL != config.GetEffectiveBaseURL() {
		result.Status = StatusWarning
		result.Message = "Authenticated with different configuration"
		result.Suggestion = "Run 'xf auth login' to re-authenticate"
		d.results = append(d.results, result)
		return
	}

	if token.IsExpired() {
		result.Status = StatusWarning
		result.Message = "Authentication token has expired"
		result.Suggestion = "Run 'xf auth login' to re-authenticate"
	} else if token.IsExpiringSoon(10 * time.Minute) {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Token expires in %s", token.TimeUntilExpiry().Round(time.Minute))
		result.Suggestion = "Consider re-authenticating soon with 'xf auth login'"
	} else {
		result.Status = StatusOK
		result.Message = "Authenticated"
		result.Details = fmt.Sprintf("Expires in %s", token.TimeUntilExpiry().Round(time.Hour))
	}

	d.results = append(d.results, result)
}

func (d *Doctor) checkGit() {
	result := &CheckResult{
		Name: "Git",
	}

	cmd := exec.Command("git", "--version")
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

func (d *Doctor) checkDocker() {
	result := &CheckResult{
		Name: "Docker",
	}

	versionCmd := exec.Command("docker", "--version")
	versionOutput, err := versionCmd.Output()
	if err != nil {
		result.Status = StatusError
		result.Message = "Docker is not installed or not in PATH"
		result.Suggestion = "Install Docker: https://docs.docker.com/get-docker/"
		d.results = append(d.results, result)
		return
	}

	infoCmd := exec.Command("docker", "info")
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

	cacheDir, err := config.DefaultCacheDir()
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to determine cache directory"
		result.Details = err.Error()
		d.results = append(d.results, result)
		return
	}

	cfg, err := config.Load()
	if err == nil && cfg.CachePath != "" {
		cacheDir = cfg.CachePath
	}

	info, err := os.Stat(cacheDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			result.Status = StatusError
			result.Message = "Cannot create cache directory"
			result.Details = fmt.Sprintf("Path: %s\nError: %s", cacheDir, err.Error())
			result.Suggestion = "Check permissions for the parent directory"
		} else {
			result.Status = StatusOK
			result.Message = fmt.Sprintf("Cache directory created: %s", cacheDir)
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
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		result.Status = StatusError
		result.Message = "Cache directory is not writable"
		result.Details = cacheDir
		result.Suggestion = "Check directory permissions"
	} else {
		os.Remove(testFile)
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Cache directory: %s", cacheDir)
	}

	d.results = append(d.results, result)
}

func (d *Doctor) checkDiskSpace() {
	result := &CheckResult{
		Name: "Disk Space",
	}

	cacheDir, err := config.DefaultCacheDir()
	if err != nil {
		result.Status = StatusSkipped
		result.Message = "Could not determine cache directory"
		d.results = append(d.results, result)
		return
	}

	// Use df command to check disk space (works on macOS and Linux).
	cmd := exec.Command("df", "-k", cacheDir)
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
			fmt.Sscanf(fields[3], "%d", &available)
		}
	}

	// available is in 1K blocks, convert to bytes.
	availableBytes := available * 1024
	const minRequired = 1 * 1024 * 1024 * 1024 // 1 GB

	if availableBytes < minRequired {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Low disk space: %s available", formatBytes(availableBytes))
		result.Suggestion = "XenForo downloads can be large. Consider freeing up disk space."
	} else {
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Available disk space: %s", formatBytes(availableBytes))
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
		{"XenForo", config.GetEffectiveBaseURL()},
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

		req, err := http.NewRequestWithContext(ctx, "HEAD", target.url, nil)
		if err != nil {
			details = append(details, fmt.Sprintf("%s: failed to create request", target.name))
			allOK = false
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			details = append(details, fmt.Sprintf("%s: unreachable", target.name))
			allOK = false
			continue
		}
		resp.Body.Close()

		details = append(details, fmt.Sprintf("%s: OK", target.name))
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

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
