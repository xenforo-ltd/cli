// Package extract provides utilities for extracting zip files.
package extract

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// errInvalidArchive indicates the archive is invalid or contains unsafe entries.
var errInvalidArchive = errors.New("invalid archive")

const maxFileSize = 32 * 1024 * 1024 // 32 MB

func extractFile(file *zip.File, root *os.Root, destPath string) error {
	if file.UncompressedSize64 > maxFileSize {
		return fmt.Errorf("file %s is too large to extract: %w", file.Name, errInvalidArchive)
	}

	if isSymlink(file) {
		return fmt.Errorf("symlink entries are not allowed in archive: %s: %w", file.Name, errInvalidArchive)
	}

	parentDir := filepath.Dir(destPath)
	if err := root.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %s: %w", parentDir, err)
	}

	srcFile, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in archive: %s: %w", file.Name, err)
	}
	defer srcFile.Close()

	limitedReader := io.LimitReader(srcFile, maxFileSize)

	destFile, err := root.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return fmt.Errorf("failed to create file: %s: %w", destPath, err)
	}
	defer destFile.Close()

	written, err := io.Copy(destFile, limitedReader)
	if err != nil {
		return fmt.Errorf("failed to write file: %s: %w", destPath, err)
	}

	if written > maxFileSize {
		return fmt.Errorf("file %s is too large to extract: %w", file.Name, errInvalidArchive)
	}

	if err := destFile.Close(); err != nil {
		return fmt.Errorf("failed to close file: %s: %w", destPath, err)
	}

	return nil
}

func sanitizePath(name string) (string, error) {
	if strings.Contains(name, "\\") || hasDriveLetter(name) || !filepath.IsLocal(name) {
		return "", fmt.Errorf("invalid path in archive: %s: %w", name, errInvalidArchive)
	}

	name = filepath.Clean(name)
	if strings.HasPrefix(name, "..") {
		return "", fmt.Errorf("invalid path in archive: %s: %w", name, errInvalidArchive)
	}

	return name, nil
}

func hasDriveLetter(name string) bool {
	if len(name) < 2 || name[1] != ':' {
		return false
	}

	c := name[0]

	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSymlink(file *zip.File) bool {
	return file.Mode()&os.ModeSymlink != 0
}

// XenForoZip extracts a XenForo ZIP file to the destination.
// It extracts only files from within the "upload/" directory, stripping that prefix.
// This handles XenForo's ZIP structure where files are under upload/ but there may
// be other files like README at the root.
func XenForoZip(zipPath, destDir string, onProgress func(current, total int, filename string)) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	root, err := os.OpenRoot(destDir)
	if err != nil {
		return fmt.Errorf("failed to open destination directory: %w", err)
	}
	defer root.Close()

	var uploadFiles []*zip.File

	for _, file := range reader.File {
		if strings.HasPrefix(filepath.ToSlash(file.Name), "upload/") {
			uploadFiles = append(uploadFiles, file)
		}
	}

	total := len(uploadFiles)

	for i, file := range uploadFiles {
		if isSymlink(file) {
			return fmt.Errorf("symlink entries are not allowed in archive: %s: %w", file.Name, errInvalidArchive)
		}

		name := strings.TrimPrefix(filepath.ToSlash(file.Name), "upload/")
		if name == "" {
			continue
		}

		destPath, err := sanitizePath(name)
		if err != nil {
			return err
		}

		if onProgress != nil {
			onProgress(i+1, total, name)
		}

		if file.FileInfo().IsDir() {
			if err := root.MkdirAll(destPath, 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %s: %w", name, err)
			}

			continue
		}

		if err := extractFile(file, root, destPath); err != nil {
			return err
		}
	}

	return nil
}
