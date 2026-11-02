package environment

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
)

// ReadEnvFile reads environment variables from an .env file.
func ReadEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf(".env file not found: %w", err)
		}

		return nil, fmt.Errorf("failed to read .env file: %w", err)
	}
	defer file.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		before, after, ok := strings.Cut(line, "=")
		if !ok {
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(before)
		value := stripQuotes(after)

		env[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading .env file: %w", err)
	}

	return env, nil
}

// stripQuotes removes surrounding single or double quotes from a string.
func stripQuotes(s string) string {
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
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create .env file: %w", err)
	}
	defer file.Close()

	for _, key := range slices.Sorted(maps.Keys(values)) {
		if _, err := fmt.Fprintf(file, "%s=%s\n", key, quoteEnvValue(values[key])); err != nil {
			return fmt.Errorf("failed to write to .env file: %w", err)
		}
	}

	return nil
}

func updateEnvFile(path string, values map[string]string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
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
			leadingSpace := line[:len(line)-len(strings.TrimLeft(line, " \t"))]

			lines[i] = fmt.Sprintf("%s%s=%s", leadingSpace, key, quoteEnvValue(newValue))
			updated[key] = true
		}
	}

	for _, key := range slices.Sorted(maps.Keys(values)) {
		if !updated[key] {
			lines = append(lines, fmt.Sprintf("%s=%s", key, quoteEnvValue(values[key])))
		}
	}

	output := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(output), 0o600); err != nil {
		return fmt.Errorf("failed to write .env file: %w", err)
	}

	return nil
}

// quoteEnvValue quotes values that contain spaces or special characters.
func quoteEnvValue(value string) string {
	if needsQuoting(value) {
		return fmt.Sprintf("\"%s\"", value)
	}

	return value
}

func needsQuoting(value string) bool {
	if strings.Contains(value, "${") {
		return false
	}

	return strings.ContainsAny(value, " \t\n\"'`$\\")
}

// MaxInstanceNameLength bounds every generated Docker instance name.
//
// Instance names become Compose project names, container and volume name
// prefixes, image tags and hostnames. The tightest of those is a DNS label, at
// 63 characters; 48 leaves room for that while keeping URLs and container names
// short enough to read.
const MaxInstanceNameLength = 48

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateInstanceName generates a Docker-safe instance name from a directory name.
func GenerateInstanceName(dirName string) string {
	name := strings.ToLower(dirName)

	name = nonAlphanumeric.ReplaceAllString(name, "-")

	name = strings.Trim(name, "-")

	if name == "" {
		name = "xf"
	}

	if len(name) > MaxInstanceNameLength {
		name = name[:MaxInstanceNameLength]
	}

	return name
}
