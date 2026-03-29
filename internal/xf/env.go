// Package xf provides utilities for XenForo installation detection and management.
package xf

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

// EnvConfig represents the configuration values in a .env file.
type EnvConfig struct {
	// Instance name (used for Docker project name)
	Instance string

	// Contexts determines which compose files to use (e.g., "nginx:mysql:development")
	Contexts string

	// Site title
	Title string

	// Admin email
	Email string

	// Contact email (defaults to Email if not set)
	ContactEmail string

	// Cookie prefix
	CookiePrefix string

	// Debug mode
	Debug bool

	// Development mode
	Development bool

	// Admin hue shift (for visual differentiation in dev)
	AdminHueShift int

	// PHP version (optional)
	PHPVersion string

	// Cache settings
	CacheSessions bool
	CachePages    bool

	// Add-on settings
	ImageMagickEnable bool
	FFMPEGEnable      bool
}

// ReadEnvFile reads environment variables from an .env file.
func ReadEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, clierrors.New(clierrors.CodeFileNotFound, ".env file not found")
		}

		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to read .env file", err)
	}
	defer file.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		before, after, ok := strings.Cut(line, "=")
		if !ok {
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(before)
		value := StripQuotes(after)

		env[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "error reading .env file", err)
	}

	return env, nil
}

// StripQuotes removes surrounding single or double quotes from a string.
func StripQuotes(s string) string {
	value := strings.TrimSpace(s)

	slen := len(value)
	if slen < 2 {
		return value
	}

	if (value[0] != '"' || value[slen-1] != '"') &&
		(value[0] != '\'' || value[slen-1] != '\'') {
		return value
	}

	return value[1 : slen-1]
}

// WriteEnvFile writes values to an .env file.
// It preserves comments and formatting from an existing file if present.
func WriteEnvFile(path string, values map[string]string) error {
	if _, err := os.Stat(path); err == nil {
		return updateEnvFile(path, values)
	}

	return createEnvFile(path, values)
}

func createEnvFile(path string, values map[string]string) error {
	file, err := os.Create(path)
	if err != nil {
		return clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to create .env file", err)
	}
	defer file.Close()

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		value := values[key]
		// Quote values that contain spaces or special characters
		if needsQuoting(value) {
			value = fmt.Sprintf("\"%s\"", value)
		}

		if _, err := fmt.Fprintf(file, "%s=%s\n", key, value); err != nil {
			return clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to write to .env file", err)
		}
	}

	return nil
}

func updateEnvFile(path string, values map[string]string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to read .env file", err)
	}

	lines := strings.Split(string(content), "\n")
	updated := make(map[string]bool)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		before, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}

		key := strings.TrimSpace(before)
		if newValue, ok := values[key]; ok {
			var leadingSpace strings.Builder

			for _, c := range line {
				if c == ' ' || c == '\t' {
					leadingSpace.WriteRune(c)
				} else {
					break
				}
			}

			if needsQuoting(newValue) {
				newValue = fmt.Sprintf("\"%s\"", newValue)
			}

			lines[i] = fmt.Sprintf("%s%s=%s", leadingSpace.String(), key, newValue)
			updated[key] = true
		}
	}

	for key, value := range values {
		if !updated[key] {
			if needsQuoting(value) {
				value = fmt.Sprintf("\"%s\"", value)
			}

			lines = append(lines, fmt.Sprintf("%s=%s", key, value))
		}
	}

	output := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(output), 0o600); err != nil {
		return clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to write .env file", err)
	}

	return nil
}

func needsQuoting(value string) bool {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		return false
	}

	if strings.Contains(value, "${") {
		return false
	}

	return strings.ContainsAny(value, " \t\n\"'`$\\")
}

// ConfigureEnv writes environment configuration to the .env file.
func (c *EnvConfig) ConfigureEnv(envPath string) error {
	values := make(map[string]string)

	if c.Instance != "" {
		values["XF_INSTANCE"] = c.Instance
	}

	if c.Contexts != "" {
		values["XF_CONTEXTS"] = c.Contexts
	}

	if c.Title != "" {
		values["XF_TITLE"] = c.Title
	}

	if c.Email != "" {
		values["XF_EMAIL"] = c.Email
	}

	if c.ContactEmail != "" {
		values["XF_CONTACT_EMAIL"] = c.ContactEmail
	}

	if c.CookiePrefix != "" {
		values["XF_COOKIE_PREFIX"] = c.CookiePrefix
	}

	if c.Debug {
		values["XF_DEBUG"] = "1"
	} else {
		values["XF_DEBUG"] = "0"
	}

	if c.Development {
		values["XF_DEVELOPMENT"] = "1"
	} else {
		values["XF_DEVELOPMENT"] = "0"
	}

	if c.CacheSessions {
		values["XF_CACHE_SESSIONS"] = "1"
	} else {
		values["XF_CACHE_SESSIONS"] = "0"
	}

	if c.CachePages {
		values["XF_CACHE_PAGES"] = "1"
	} else {
		values["XF_CACHE_PAGES"] = "0"
	}

	if c.ImageMagickEnable {
		values["XF_IMAGICK_ENABLE"] = "1"
	}

	if c.FFMPEGEnable {
		values["XF_XFMG_FFMPEG_ENABLE"] = "1"
	}

	if c.AdminHueShift != 0 {
		values["XF_ADMIN_HUE_SHIFT"] = strconv.Itoa(c.AdminHueShift)
	}

	if c.PHPVersion != "" {
		values["PHP_VERSION"] = c.PHPVersion
	}

	return WriteEnvFile(envPath, values)
}

// GetEnvPath returns the path to the .env file in a XenForo directory.
func GetEnvPath(xfDir string) string {
	return filepath.Join(xfDir, ".env")
}

// GetXenForoDir finds the XenForo root directory by traversing up from startDir.
// It also checks the XF_DIR environment variable as a fallback.
func GetXenForoDir(startDir string) (string, error) {
	dir := filepath.Clean(startDir)
	for {
		xfPath := filepath.Join(dir, "src", "XF.php")
		if _, err := os.Stat(xfPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	if xfDir := filepath.Clean(os.Getenv("XF_DIR")); xfDir != "." {
		if _, err := os.Stat(filepath.Join(xfDir, "src", "XF.php")); err == nil {
			return xfDir, nil
		}
	}

	return "", clierrors.New(clierrors.CodeInvalidInput, "not in a XenForo directory and XF_DIR not set")
}

// GenerateInstanceName generates a Docker-safe instance name from a directory name.
func GenerateInstanceName(dirName string) string {
	name := strings.ToLower(dirName)

	reg := regexp.MustCompile(`[^a-z0-9]+`)
	name = reg.ReplaceAllString(name, "-")

	name = strings.Trim(name, "-")

	if name == "" {
		name = "xf"
	}

	if len(name) > 32 {
		name = name[:32]
	}

	return name
}
