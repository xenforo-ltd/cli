package embed

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

//go:embed docker/*
var dockerFS embed.FS

const DockerDir = "docker"

// ExtractDockerFiles extracts all embedded Docker files to the target directory.
// Base files are always overwritten. Default files (.env, .dockerignore) are
// copied with .default suffix if they already exist and differ.
func ExtractDockerFiles(targetDir string) error {
	return extractDir(DockerDir, targetDir, "", true)
}

// ExtractDockerFilesWithOptions extracts Docker files with custom options.
type ExtractOptions struct {
	OverwriteBaseFiles bool
	Contexts           []string
}

// ExtractDockerFilesWithOptions extracts Docker files with custom options.
func ExtractDockerFilesWithOptions(targetDir string, opts ExtractOptions) error {
	if err := extractDir(DockerDir, targetDir, "", opts.OverwriteBaseFiles); err != nil {
		return err
	}

	if len(opts.Contexts) > 0 {
		if err := filterComposeFiles(targetDir, opts.Contexts); err != nil {
			return err
		}
	}

	return nil
}

func extractDir(srcDir, targetDir, relPath string, overwriteBaseFiles bool) error {
	entries, err := dockerFS.ReadDir(srcDir)
	if err != nil {
		return clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to read embedded directory", err)
	}

	for _, entry := range entries {
		srcPath := path.Join(srcDir, entry.Name())
		targetPath := filepath.Join(targetDir, relPath, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return clierrors.Wrap(clierrors.CodeDirCreateFailed, "failed to create directory", err)
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
		return clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to read embedded file", err)
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return clierrors.Wrap(clierrors.CodeDirCreateFailed, "failed to create parent directory", err)
	}

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to write file", err)
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
		return clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to read embedded default file", err)
	}

	parentDir := filepath.Dir(targetBase)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return clierrors.Wrap(clierrors.CodeDirCreateFailed, "failed to create parent directory", err)
	}

	if existingData, err := os.ReadFile(targetBase); err == nil {
		if string(existingData) != string(data) {
			defaultPath := targetBase + ".default"
			if err := os.WriteFile(defaultPath, data, 0644); err != nil {
				return clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to write default file", err)
			}
		}
		return nil
	}

	if err := os.WriteFile(targetBase, data, 0644); err != nil {
		return clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to write file", err)
	}

	return nil
}

// GetDockerFile returns the contents of an embedded Docker file.
func GetDockerFile(name string) ([]byte, error) {
	embeddedPath := path.Join(DockerDir, name)
	data, err := dockerFS.ReadFile(embeddedPath)
	if err != nil {
		return nil, clierrors.Wrapf(clierrors.CodeFileReadFailed, err, "failed to read embedded file: %s", name)
	}
	return data, nil
}

// ListComposeFiles returns a list of all compose file names.
func ListComposeFiles() []string {
	var files []string
	entries, err := dockerFS.ReadDir(DockerDir)
	if err != nil {
		return files
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "compose") && strings.HasSuffix(entry.Name(), ".yaml") {
			files = append(files, entry.Name())
		}
	}

	return files
}

// GetEnvDefault returns the default .env file content.
func GetEnvDefault() ([]byte, error) {
	return GetDockerFile(".env.default")
}

// GetDockerIgnoreDefault returns the default .dockerignore file content.
func GetDockerIgnoreDefault() ([]byte, error) {
	return GetDockerFile(".dockerignore.default")
}

// CopyEmbeddedFile copies a specific embedded file to a target path.
func CopyEmbeddedFile(embeddedName, targetPath string) error {
	data, err := GetDockerFile(embeddedName)
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return clierrors.Wrap(clierrors.CodeDirCreateFailed, "failed to create parent directory", err)
	}

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return clierrors.Wrap(clierrors.CodeFileWriteFailed, "failed to write file", err)
	}

	return nil
}

// ListEmbeddedFiles returns all embedded Docker file paths.
func ListEmbeddedFiles() ([]string, error) {
	var files []string

	err := fs.WalkDir(dockerFS, DockerDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			relPath := strings.TrimPrefix(path, DockerDir+"/")
			files = append(files, relPath)
		}
		return nil
	})

	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to list embedded files", err)
	}

	return files, nil
}

// filterComposeFiles is currently a no-op because contexts are selected via XF_CONTEXTS.
func filterComposeFiles(targetDir string, contexts []string) error {
	return nil
}

// ExtractToWriter writes an embedded file to the provided writer.
func ExtractToWriter(embeddedName string, w io.Writer) error {
	data, err := GetDockerFile(embeddedName)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// GetFileInfo returns information about an embedded file.
func GetFileInfo(name string) (fs.FileInfo, error) {
	embeddedPath := path.Join(DockerDir, name)
	file, err := dockerFS.Open(embeddedPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return file.Stat()
}

// ValidateEmbeddedFiles checks that all expected Docker files are embedded.
func ValidateEmbeddedFiles() error {
	requiredFiles := []string{
		"compose.yaml",
		"Dockerfile",
		".env.default",
		".dockerignore.default",
		"src/config.docker.php",
	}

	for _, file := range requiredFiles {
		if _, err := GetDockerFile(file); err != nil {
			return clierrors.Newf(clierrors.CodeFileNotFound, "required embedded file missing: %s", file)
		}
	}

	return nil
}

// String returns a string representation of all embedded files (for debugging).
func String() string {
	files, err := ListEmbeddedFiles()
	if err != nil {
		return fmt.Sprintf("Error listing files: %v", err)
	}
	return fmt.Sprintf("Embedded Docker files:\n  %s", strings.Join(files, "\n  "))
}
