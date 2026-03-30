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
func ExtractDockerFilesWithOptions(targetDir string, opts ExtractOptions) error {
	if err := extractDir(EmbedDir, targetDir, "", opts.OverwriteBaseFiles); err != nil {
		return err
	}

	return nil
}

func extractDir(srcDir, targetDir, relPath string, overwriteBaseFiles bool) error {
	entries, err := dockerFS.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("failed to read embedded directory: %w", err)
	}

	for _, entry := range entries {
		srcPath := path.Join(srcDir, entry.Name())
		targetPath := filepath.Join(targetDir, relPath, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(targetPath, 0o750); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

			if err := extractDir(srcPath, targetDir, filepath.Join(relPath, entry.Name()), overwriteBaseFiles); err != nil {
				return err
			}

			continue
		}

		if isDefaultFile(entry.Name()) {
			if err := extractDefaultFile(srcPath, targetPath); err != nil {
				return err
			}

			continue
		}

		if !overwriteBaseFiles {
			if _, err := os.Stat(targetPath); err == nil {
				continue
			}
		}

		if err := extractFile(srcPath, targetPath); err != nil {
			return err
		}
	}

	return nil
}

func extractFile(srcPath, targetPath string) error {
	data, err := dockerFS.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read embedded file: %w", err)
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0o750); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if err := os.WriteFile(targetPath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func isDefaultFile(name string) bool {
	return strings.HasSuffix(name, ".default")
}

func extractDefaultFile(srcPath, targetPath string) error {
	targetBase := strings.TrimSuffix(targetPath, ".default")

	data, err := dockerFS.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("failed to read embedded default file: %w", err)
	}

	parentDir := filepath.Dir(targetBase)
	if err := os.MkdirAll(parentDir, 0o750); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if existingData, err := os.ReadFile(targetBase); err == nil {
		if string(existingData) != string(data) {
			defaultPath := targetBase + ".default"
			if err := os.WriteFile(defaultPath, data, 0o600); err != nil {
				return fmt.Errorf("failed to write default file: %w", err)
			}
		}

		return nil
	}

	if err := os.WriteFile(targetBase, data, 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
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
