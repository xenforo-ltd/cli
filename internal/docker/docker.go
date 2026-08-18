// Package docker contains embedded Docker files and extraction utilities.
package docker

import (
	"embed"
	"fmt"
	"io/fs"
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
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
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
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
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
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if existingData, err := os.ReadFile(targetBase); err == nil {
		if string(existingData) != string(data) {
			defaultPath := targetBase + ".default"
			if err := os.WriteFile(defaultPath, data, 0o644); err != nil {
				return fmt.Errorf("failed to write default file: %w", err)
			}
		}

		return nil
	}

	if err := os.WriteFile(targetBase, data, defaultFileMode(targetBase)); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// defaultFileMode returns the mode for a file generated from an embedded
// ".default" companion. The .env file is kept private because it may contain
// secrets; all other generated configuration is world-readable so it can be
// consumed by containers.
func defaultFileMode(targetPath string) os.FileMode {
	if filepath.Base(targetPath) == ".env" {
		return 0o600
	}

	return 0o644
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

// ListEmbeddedFiles returns the repository-relative paths that extraction
// writes into a XenForo directory, in no particular order.
//
// The list is derived from the embedded tree rather than hardcoded, so it
// cannot drift as files are added to or removed from it. ".default" files are
// reported under the name they are written as, matching extractDefaultFile.
func ListEmbeddedFiles() ([]string, error) {
	var paths []string

	err := fs.WalkDir(dockerFS, EmbedDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(EmbedDir, filepath.FromSlash(p))
		if err != nil {
			return fmt.Errorf("failed to resolve embedded path %s: %w", p, err)
		}

		paths = append(paths, strings.TrimSuffix(rel, ".default"))

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list embedded files: %w", err)
	}

	return paths, nil
}
