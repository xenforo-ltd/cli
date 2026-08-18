// Package docker contains embedded Docker files and extraction utilities.
package docker

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed embed/*
var dockerFS embed.FS

// EmbedDir is the directory containing embedded Docker configuration files.
const EmbedDir = "embed"

// ExtractOptions specifies options for extracting Docker files.
type ExtractOptions struct {
	OverwriteBaseFiles bool
	Contexts           []string
}

// ExtractDockerFilesWithOptions extracts Docker files with custom options.
// It returns the paths of any ".default" files written alongside existing,
// user-modified files so callers can notify the user.
func ExtractDockerFilesWithOptions(targetDir string, opts ExtractOptions) (written []string, err error) {
	written, err = extractDir(EmbedDir, targetDir, "", opts.OverwriteBaseFiles)
	if err != nil {
		return nil, err
	}

	return written, nil
}

func extractDir(srcDir, targetDir, relPath string, overwriteBaseFiles bool) ([]string, error) {
	entries, err := dockerFS.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded directory %s: %w", srcDir, err)
	}

	var written []string

	for _, entry := range entries {
		srcPath := path.Join(srcDir, entry.Name())
		targetPath := filepath.Join(targetDir, relPath, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(targetPath, 0o750); err != nil {
				return nil, fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}

			childWritten, err := extractDir(srcPath, targetDir, filepath.Join(relPath, entry.Name()), overwriteBaseFiles)
			if err != nil {
				return nil, err
			}

			written = append(written, childWritten...)

			continue
		}

		if isDefaultFile(entry.Name()) {
			defaultPath, err := extractDefaultFile(srcPath, targetPath)
			if err != nil {
				return nil, err
			}

			if defaultPath != "" {
				written = append(written, defaultPath)
			}

			continue
		}

		if !overwriteBaseFiles {
			if _, err := os.Stat(targetPath); err == nil {
				continue
			}
		}

		if err := extractFile(srcPath, targetPath); err != nil {
			return nil, err
		}
	}

	return written, nil
}

func extractFile(srcPath, targetPath string) error {
	data, err := dockerFS.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read embedded file %s: %w", srcPath, err)
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0o750); err != nil {
		return fmt.Errorf("failed to create parent directory %s: %w", parentDir, err)
	}

	if err := os.WriteFile(targetPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", targetPath, err)
	}

	return nil
}

func isDefaultFile(name string) bool {
	return strings.HasSuffix(name, ".default")
}

// extractDefaultFile writes the embedded default alongside an existing,
// user-modified file. It returns the path written, or "" if nothing was
// written (either the target didn't exist yet and was created directly, or
// the existing file matches the embedded content).
func extractDefaultFile(srcPath, targetPath string) (string, error) {
	targetBase := strings.TrimSuffix(targetPath, ".default")

	data, err := dockerFS.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("failed to read embedded default file %s: %w", srcPath, err)
	}

	parentDir := filepath.Dir(targetBase)
	if err := os.MkdirAll(parentDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create parent directory %s: %w", parentDir, err)
	}

	if existingData, err := os.ReadFile(targetBase); err == nil {
		if string(existingData) != string(data) {
			defaultPath := targetBase + ".default"
			if err := os.WriteFile(defaultPath, data, 0o600); err != nil {
				return "", fmt.Errorf("failed to write %s: %w", defaultPath, err)
			}

			return defaultPath, nil
		}

		return "", nil
	}

	if err := os.WriteFile(targetBase, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", targetBase, err)
	}

	return "", nil
}

// GetDockerFile returns the contents of an embedded Docker file.
func GetDockerFile(name string) ([]byte, error) {
	embeddedPath := path.Join(EmbedDir, name)

	data, err := dockerFS.ReadFile(embeddedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded file: %s: %w", name, err)
	}

	return data, nil
}

// GetEnvDefault returns the default .env file content.
func GetEnvDefault() ([]byte, error) {
	return GetDockerFile(".env.default")
}

// GetDockerIgnoreDefault returns the default .dockerignore file content.
func GetDockerIgnoreDefault() ([]byte, error) {
	return GetDockerFile(".dockerignore.default")
}

// ListEmbeddedFiles returns all embedded Docker file paths.
