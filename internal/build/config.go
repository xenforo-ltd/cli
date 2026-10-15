package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// addOnIDPattern matches single-part and vendor-prefixed add-on IDs.
var addOnIDPattern = regexp.MustCompile(`(?i)^[a-z][a-z0-9]*(/[a-z][a-z0-9]*)?$`)

// releaseVersionPattern matches characters removed from release version names.
var releaseVersionPattern = regexp.MustCompile(`[^a-zA-Z0-9._ -]`)

// placeholderPattern matches metadata placeholders in command arguments.
var placeholderPattern = regexp.MustCompile(`\{([a-zA-Z_\x{7f}-\x{ff}][a-zA-Z0-9_\x{7f}-\x{ff}]*)\}`)

// config is the validated build configuration for an add-on.
type config struct {
	addOnPaths
	releaseName      string
	composerAutoload string
	whitelist        []filePattern
	blacklist        []filePattern
	hashExclude      []filePattern
	additional       []string
	minify           []filePattern
	rollups          []rollupEntry
	commands         [][]string
}

// rollupEntry describes an output script and its ordered input patterns.
type rollupEntry struct {
	output string
	inputs []filePattern
}

// loadBuildRoot reads the add-on configuration and opens its build root.
// The caller must close the returned root.
func loadBuildRoot(ctx context.Context, installation *os.Root, addOnID string) (*config, *os.Root, error) {
	cfg, err := readConfig(ctx, installation, addOnID)
	if err != nil {
		return nil, nil, err
	}

	root, err := installation.OpenRoot(filepath.FromSlash(cfg.prefix))
	if err != nil {
		return nil, nil, fmt.Errorf("opening build root: %w", err)
	}

	return cfg, root, nil
}

// readConfig loads optional build.json and required addon.json from the add-on directory.
func readConfig(ctx context.Context, installation *os.Root, addOnID string) (*config, error) {
	if !addOnIDPattern.MatchString(addOnID) {
		return nil, fmt.Errorf("invalid add-on ID: %q", addOnID)
	}

	addOnPrefix := path.Join("src", "addons", addOnID)
	buildPath := path.Join(addOnPrefix, "build.json")
	buildJSON, err := readFile(ctx, installation, buildPath)
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := installation.Lstat(filepath.FromSlash(buildPath)); errors.Is(statErr, os.ErrNotExist) {
			buildJSON, err = []byte("{}"), nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("reading build.json: %w", err)
	}

	addonJSON, err := readFile(ctx, installation, path.Join(addOnPrefix, "addon.json"))
	if err != nil {
		return nil, fmt.Errorf("reading addon.json: %w", err)
	}

	return parseConfig(buildJSON, addonJSON, addOnID)
}

// parseConfig validates build settings and metadata, resolves paths, and expands
// command placeholders.
func parseConfig(buildJSON, addonJSON []byte, addOnID string) (*config, error) {
	addOnPrefix := path.Join("src", "addons", addOnID)
	raw := struct {
		Root        string            `json:"root"`
		Whitelist   []string          `json:"whitelist"`
		Blacklist   []string          `json:"blacklist"`
		HashExclude []string          `json:"hash_exclude"`
		Additional  []string          `json:"additional_files"`
		Minify      json.RawMessage   `json:"minify"`
		Rollup      json.RawMessage   `json:"rollup"`
		Exec        []json.RawMessage `json:"exec"`
	}{Root: "."}

	if err := json.Unmarshal(buildJSON, &raw); err != nil {
		return nil, fmt.Errorf("parsing build.json: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(buildJSON), []byte("null")) {
		return nil, errors.New("parsing build.json: expected an object")
	}

	root, err := relativePath(raw.Root, rootPath)
	if err != nil {
		return nil, fmt.Errorf("invalid build.json root: %w", err)
	}

	prefix := path.Join(addOnPrefix, root)
	if prefix == ".." || strings.HasPrefix(prefix, "../") {
		return nil, errors.New("build.json root must stay within the XenForo installation")
	}

	if prefix != "." && prefix != addOnPrefix && !strings.HasPrefix(addOnPrefix, prefix+"/") {
		return nil, errors.New("build.json root must contain the add-on directory")
	}

	addOnDir, _ := within(addOnPrefix, prefix)

	cfg := &config{addOnDir: addOnDir, prefix: prefix}
	if len(raw.Whitelist) == 0 {
		raw.Whitelist = []string{"**"}
	}

	if cfg.whitelist, err = parsePatterns(raw.Whitelist); err != nil {
		return nil, fmt.Errorf("invalid whitelist: %w", err)
	}

	if cfg.blacklist, err = parsePatterns(raw.Blacklist); err != nil {
		return nil, fmt.Errorf("invalid blacklist: %w", err)
	}
	if cfg.hashExclude, err = parsePatterns(raw.HashExclude); err != nil {
		return nil, fmt.Errorf("invalid hash_exclude: %w", err)
	}

	if cfg.minify, err = parseMinify(raw.Minify); err != nil {
		return nil, fmt.Errorf("invalid minify: %w", err)
	}

	if cfg.rollups, err = parseRollup(raw.Rollup); err != nil {
		return nil, err
	}

	for _, entry := range raw.Additional {
		entry, err := relativePath(entry, literalPath)
		if err != nil {
			return nil, fmt.Errorf("invalid additional file: %w", err)
		}

		cfg.additional = append(cfg.additional, entry)
	}

	metadata, err := parseMetadata(addonJSON)
	if err != nil {
		return nil, err
	}
	if value := metadata["composer_autoload"]; value != nil {
		autoload, ok := value.(string)
		if !ok {
			return nil, errors.New("addon.json composer_autoload must be a string")
		}
		if autoload != "" {
			cfg.composerAutoload, err = relativePath(autoload, literalPath)
			if err != nil {
				return nil, fmt.Errorf("invalid addon.json composer_autoload: %w", err)
			}
		}
	}

	versionString, _ := metadata["version_string"].(string)
	version := strings.Join(strings.Fields(releaseVersionPattern.ReplaceAllString(versionString, "")), " ")
	if version == "" {
		return nil, errors.New("addon.json version_string must contain a usable release version")
	}

	cfg.releaseName = strings.ReplaceAll(addOnID, "/", "-") + "-" + version + ".zip"
	for i, entry := range raw.Exec {
		var command []string
		if err := json.Unmarshal(entry, &command); err != nil {
			var line string
			if err := json.Unmarshal(entry, &line); err != nil {
				return nil, fmt.Errorf("build.json exec command %d must be a string or an array of strings", i+1)
			}
			command = strings.Fields(line)
		}

		if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
			return nil, errors.New("build.json exec commands must not be empty")
		}

		expanded := make([]string, len(command))
		for i, argument := range command {
			expanded[i] = expandArgument(argument, metadata, addOnID)
		}

		cfg.commands = append(cfg.commands, expanded)
	}

	return cfg, nil
}

// parseMinify decodes "*" or a list of glob patterns.
// An absent or null value disables minification.
func parseMinify(raw json.RawMessage) ([]filePattern, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	var wildcard string
	if err := json.Unmarshal(raw, &wildcard); err == nil {
		if wildcard != "*" {
			return nil, fmt.Errorf("minify must be \"*\" or a list of globs, not %q", wildcard)
		}

		return parsePatterns([]string{"js/**/*.js"})
	}

	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, errors.New("minify must be \"*\" or a list of globs")
	}

	return parsePatterns(values)
}

// parseRollup decodes rollup entries in declaration order.
// An absent or null value produces no entries.
func parseRollup(raw json.RawMessage) ([]rollupEntry, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("parsing rollup: %w", err)
	}

	if token != json.Delim('{') {
		return nil, errors.New("rollup must be an object mapping output paths to input lists")
	}

	var entries []rollupEntry
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("parsing rollup output: %w", err)
		}

		output, ok := token.(string)
		if !ok {
			return nil, errors.New("rollup output must be a string")
		}

		var inputs []string
		if err := decoder.Decode(&inputs); err != nil {
			return nil, fmt.Errorf("rollup output %q: %w", output, err)
		}

		original := output
		output, err = relativePath(output, literalPath)
		if err != nil {
			return nil, fmt.Errorf("invalid rollup output: %w", err)
		}

		if !isLiteral(output) || !strings.HasSuffix(output, ".js") {
			return nil, fmt.Errorf("rollup output %q must be a relative path ending in .js", output)
		}

		if seen[output] {
			return nil, fmt.Errorf("rollup output %q duplicates %q", original, output)
		}

		seen[output] = true
		patterns, err := parsePatterns(inputs)
		if err != nil {
			return nil, fmt.Errorf("rollup output %q: %w", output, err)
		}

		if len(patterns) == 0 {
			return nil, fmt.Errorf("rollup output %q has no inputs", output)
		}

		entries = append(entries, rollupEntry{output: output, inputs: patterns})
	}

	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("parsing rollup: %w", err)
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("unexpected data after rollup object")
	}

	return entries, nil
}

// expandArgument substitutes scalar metadata and the add-on ID in argument.
// Unknown placeholders and empty string values are left unchanged.
func expandArgument(argument string, metadata map[string]any, addOnID string) string {
	return placeholderPattern.ReplaceAllStringFunc(argument, func(match string) string {
		name := match[1 : len(match)-1]
		value := metadata[name]
		if name == "addon_id" {
			value = addOnID
		}

		switch value := value.(type) {
		case string:
			if value != "" {
				return value
			}

		case json.Number:
			return value.String()
		case bool:
			return strconv.FormatBool(value)
		}

		return match
	})
}
