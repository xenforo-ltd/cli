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

// ErrInvalidArchive indicates the archive is invalid or contains unsafe entries.
var ErrInvalidArchive = errors.New("invalid archive")

const maxFileSize = 32 * 1024 * 1024 // 32 MB

func extractFile(file *zip.File, destPath string) error {
	if file.UncompressedSize64 > maxFileSize {
		return fmt.Errorf("file %s is too large to extract: %w", file.Name, ErrInvalidArchive)
	}

	if isSymlink(file) {
		return fmt.Errorf("symlink entries are not allowed in archive: %s: %w", file.Name, ErrInvalidArchive)
	}

	parentDir := filepath.Dir(destPath)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %s: %w", parentDir, err)
	}

	srcFile, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in archive: %s: %w", file.Name, err)
	}
	defer srcFile.Close()

	limitedReader := io.LimitReader(srcFile, maxFileSize)

	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return fmt.Errorf("failed to create file: %s: %w", destPath, err)
	}
	defer destFile.Close()

	written, err := io.Copy(destFile, limitedReader)
	if err != nil {
		return fmt.Errorf("failed to write file: %s: %w", destPath, err)
	}

	if written > maxFileSize {
		return fmt.Errorf("file %s is too large to extract: %w", file.Name, ErrInvalidArchive)
	}

	if err := destFile.Close(); err != nil {
		return fmt.Errorf("failed to close file: %s: %w", destPath, err)
	}

	return nil
}

// sanitizePath ensures the path is safe and within the destination directory.
// This prevents "zip slip" directory traversal attacks.
func sanitizePath(destDir, name string) (string, error) {
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return "", fmt.Errorf("invalid path in archive: %s: %w", name, ErrInvalidArchive)
	}

	if strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid path in archive: %s: %w", name, ErrInvalidArchive)
	}

	if len(name) >= 2 {
		c := name[0]
		if name[1] == ':' && ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return "", fmt.Errorf("invalid path in archive: %s: %w", name, ErrInvalidArchive)
		}
	}

	if filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid path in archive: %s: %w", name, ErrInvalidArchive)
	}

	name = filepath.Clean(name)

	destPath := filepath.Join(destDir, name)

	cleanDestDir := filepath.Clean(destDir)
	cleanDestPath := filepath.Clean(destPath)

	rel, err := filepath.Rel(cleanDestDir, cleanDestPath)
	if err != nil {
		return "", fmt.Errorf("invalid path in archive: %w", err)
	}

	if rel == "." {
		return cleanDestPath, nil
	}

	if strings.HasPrefix(rel, "..") || rel == "" {
		return "", fmt.Errorf("invalid path in archive: %s: %w", name, ErrInvalidArchive)
	}

	return destPath, nil
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

	var uploadFiles []*zip.File

	for _, file := range reader.File {
		name := filepath.ToSlash(file.Name)
		if strings.HasPrefix(name, "upload/") {
			uploadFiles = append(uploadFiles, file)
		}
	}

	total := len(uploadFiles)
	current := 0

	for _, file := range uploadFiles {
		current++

		if isSymlink(file) {
			return fmt.Errorf("symlink entries are not allowed in archive: %s: %w", file.Name, ErrInvalidArchive)
		}

		name := strings.TrimPrefix(filepath.ToSlash(file.Name), "upload/")
		if name == "" {
			continue
		}

		destPath, err := sanitizePath(destDir, name)
		if err != nil {
			return err
		}

		if onProgress != nil {
			onProgress(current, total, name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return fmt.Errorf("failed to create directory: %s: %w", name, err)
			}

			continue
		}

		if err := extractFile(file, destPath); err != nil {
			return err
		}
	}

	return nil
}
