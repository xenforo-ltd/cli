package environment

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
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
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create .env file: %w", err)
	}
	defer file.Close()

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	for _, key := range keys {
		value := values[key]
		// Quote values that contain spaces or special characters
		if needsQuoting(value) {
			value = fmt.Sprintf("\"%s\"", value)
		}

		if _, err := fmt.Fprintf(file, "%s=%s\n", key, value); err != nil {
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
		return fmt.Errorf("failed to write .env file: %w", err)
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

// MaxInstanceNameLength bounds every generated Docker instance name.
//
// Instance names become Compose project names, container and volume name
// prefixes, image tags and hostnames. The tightest of those is a DNS label, at
// 63 characters; 48 leaves room for that while keeping URLs and container names
// short enough to read.
const MaxInstanceNameLength = 48

// GenerateInstanceName generates a Docker-safe instance name from a directory name.
func GenerateInstanceName(dirName string) string {
	name := strings.ToLower(dirName)

	reg := regexp.MustCompile(`[^a-z0-9]+`)
	name = reg.ReplaceAllString(name, "-")

	name = strings.Trim(name, "-")

	if name == "" {
		name = "xf"
	}

	if len(name) > MaxInstanceNameLength {
		name = name[:MaxInstanceNameLength]
	}

	return name
}
