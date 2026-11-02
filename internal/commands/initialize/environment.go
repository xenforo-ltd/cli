package initialize

import (
	"bufio"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"strings"
)

var (
	// envKeyPattern restricts environment keys to uppercase letters, digits and underscores.
	envKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	// errInvalidEnvFormat reports an assignment without an equals sign.
	errInvalidEnvFormat = errors.New("expected KEY=VALUE")
	// errInvalidEnvKey reports a key outside the allowed syntax.
	errInvalidEnvKey = errors.New("must match ^[A-Z][A-Z0-9_]*$")
	// errNewlinesNotAllowed reports a multiline environment value.
	errNewlinesNotAllowed = errors.New("newlines are not allowed")
)

// parseEnvFile parses environment variables from a file.
func parseEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file %s: %w", path, err)
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

		key, value, err := parseEnvAssignment(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid env line %d: %w", line, err)
		}

		values[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read env file %s: %w", path, err)
	}

	return values, nil
}

// parseEnvFlags parses environment variables from command-line arguments.
func parseEnvFlags(entries []string) (map[string]string, error) {
	values := map[string]string{}

	for _, entry := range entries {
		key, value, err := parseEnvAssignment(strings.TrimSpace(entry))
		if err != nil {
			return nil, fmt.Errorf("invalid --env %q: %w", entry, err)
		}

		values[key] = value
	}

	return values, nil
}

// parseEnvAssignment splits and trims a KEY=VALUE assignment and validates its key.
func parseEnvAssignment(s string) (key, value string, err error) {
	key, value, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", errInvalidEnvFormat
	}

	key = strings.TrimSpace(key)
	if err := validateEnvKey(key); err != nil {
		return "", "", err
	}

	return key, strings.TrimSpace(value), nil
}

// validateEnvKey checks if an environment variable key is valid.
func validateEnvKey(key string) error {
	if !envKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid key %q: %w", key, errInvalidEnvKey)
	}

	return nil
}

// mergeEnvMaps combines values with flags taking precedence over file and base values.
func mergeEnvMaps(base, fromFile, fromFlags map[string]string) map[string]string {
	merged := map[string]string{}

	maps.Copy(merged, base)

	maps.Copy(merged, fromFile)

	maps.Copy(merged, fromFlags)

	return merged
}
