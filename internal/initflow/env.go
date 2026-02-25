// Package initflow provides interactive workflow builders for setup tasks.
package initflow

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ErrInvalidEnvFormat is returned when an environment entry is malformed.
var (
	envKeyPattern         = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	ErrInvalidEnvFormat   = errors.New("expected KEY=VALUE")
	ErrInvalidEnvKey      = errors.New("must match ^[A-Z][A-Z0-9_]*$")
	ErrNewlinesNotAllowed = errors.New("newlines are not allowed")
)

// ParseEnvFile parses environment variables from a file.
func ParseEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)

	line := 0
	for scanner.Scan() {
		line++

		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}

		key, value, ok := strings.Cut(raw, "=")
		if !ok {
			return nil, fmt.Errorf("invalid env line %d: %w", line, ErrInvalidEnvFormat)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if err := ValidateEnvKey(key); err != nil {
			return nil, fmt.Errorf("invalid env line %d: %w", line, err)
		}

		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return values, nil
}

// ParseEnvFlags parses environment variables from command-line arguments.
func ParseEnvFlags(entries []string) (map[string]string, error) {
	values := map[string]string{}

	for _, entry := range entries {
		key, value, ok := strings.Cut(strings.TrimSpace(entry), "=")
		if !ok {
			return nil, fmt.Errorf("invalid --env %q: %w", entry, ErrInvalidEnvFormat)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if err := ValidateEnvKey(key); err != nil {
			return nil, fmt.Errorf("invalid --env %q: %w", entry, err)
		}

		values[key] = value
	}

	return values, nil
}

// ValidateEnvKey checks if an environment variable key is valid.
func ValidateEnvKey(key string) error {
	if !envKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid key %q: %w", key, ErrInvalidEnvKey)
	}

	return nil
}

// MergeEnvMaps merges multiple environment maps with priority order.
func MergeEnvMaps(base, fromFile, fromFlags map[string]string) (map[string]string, map[string]string) {
	merged := map[string]string{}
	sources := map[string]string{}

	for k, v := range base {
		merged[k] = v
		sources[k] = "inferred"
	}

	for k, v := range fromFile {
		merged[k] = v
		sources[k] = "env-file"
	}

	for k, v := range fromFlags {
		merged[k] = v
		sources[k] = "--env"
	}

	return merged, sources
}
