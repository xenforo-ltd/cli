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

// Options configures the extraction behavior.
type Options struct {
	// StripComponents removes this many leading path components from extracted files.
	// For example, if StripComponents=1 and the archive contains "upload/src/XF.php",
	// it will be extracted as "src/XF.php".
	StripComponents int

	// OverwriteExisting allows overwriting existing files.
	OverwriteExisting bool

	// PreservePermissions preserves file permissions from the archive.
	PreservePermissions bool

	// OnProgress is called for each file extracted (if set).
	OnProgress func(current, total int, filename string)
}

// DefaultOptions returns the default extraction options.
func DefaultOptions() *Options {
	return &Options{
		StripComponents:     0,
		OverwriteExisting:   true,
		PreservePermissions: true,
	}
}

// ZipFile extracts a zip archive to a destination directory with optional processing.
func ZipFile(zipPath, destDir string, opts *Options) error {
	if opts == nil {
		opts = DefaultOptions()
	}

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	total := len(reader.File)
	current := 0

	for _, file := range reader.File {
		current++

		if isSymlink(file) {
			return fmt.Errorf("symlink entries are not allowed in archive: %s: %w", file.Name, ErrInvalidArchive)
		}

		name := file.Name
		if opts.StripComponents > 0 {
			name = stripPathComponents(name, opts.StripComponents)
			if name == "" {
				continue
			}
		}

		destPath, err := sanitizePath(destDir, name)
		if err != nil {
			return err
		}

		if opts.OnProgress != nil {
			opts.OnProgress(current, total, name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0o750); err != nil {
				return fmt.Errorf("failed to create directory: %s: %w", name, err)
			}

			continue
		}

		if err := extractFile(file, destPath, opts); err != nil {
			return err
		}
	}

	return nil
}

func extractFile(file *zip.File, destPath string, opts *Options) error {
	if file.UncompressedSize64 > maxFileSize {
		return fmt.Errorf("file %s is too large to extract: %w", file.Name, ErrInvalidArchive)
	}

	if isSymlink(file) {
		return fmt.Errorf("symlink entries are not allowed in archive: %s: %w", file.Name, ErrInvalidArchive)
	}

	if !opts.OverwriteExisting {
		if _, err := os.Stat(destPath); err == nil {
			return nil
		}
	}

	parentDir := filepath.Dir(destPath)
	if err := os.MkdirAll(parentDir, 0o750); err != nil {
		return fmt.Errorf("failed to create directory: %s: %w", parentDir, err)
	}

	srcFile, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open file in archive: %s: %w", file.Name, err)
	}
	defer srcFile.Close()

	mode := file.Mode()
	if !opts.PreservePermissions {
		mode = 0o600
	}

	limitedReader := io.LimitReader(srcFile, maxFileSize)

	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
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

	return nil
}

// For example, stripPathComponents("upload/src/XF.php", 1) returns "src/XF.php".
func stripPathComponents(path string, n int) string {
	path = filepath.ToSlash(path)

	parts := strings.Split(path, "/")
	if n >= len(parts) {
		return ""
	}

	return strings.Join(parts[n:], "/")
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

// GetZipRootDirectory returns the common root directory of all files in the ZIP.
// XenForo ZIPs typically have all files under an "upload/" directory.
func GetZipRootDirectory(zipPath string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip file: %w", err)
	}
	defer reader.Close()

	if len(reader.File) == 0 {
		return "", fmt.Errorf("zip file is empty: %w", ErrInvalidArchive)
	}

	firstFile := reader.File[0].Name

	archiveRootParts := 2

	parts := strings.SplitN(filepath.ToSlash(firstFile), "/", archiveRootParts)
	if len(parts) == 0 {
		return "", nil
	}

	root := parts[0]

	for _, file := range reader.File {
		fileParts := strings.SplitN(filepath.ToSlash(file.Name), "/", archiveRootParts)
		if len(fileParts) == 0 || fileParts[0] != root {
			return "", nil
		}
	}

	return root, nil
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

	if err := os.MkdirAll(destDir, 0o750); err != nil {
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
			if err := os.MkdirAll(destPath, 0o750); err != nil {
				return fmt.Errorf("failed to create directory: %s: %w", name, err)
			}

			continue
		}

		opts := &Options{
			OverwriteExisting:   true,
			PreservePermissions: true,
		}
		if err := extractFile(file, destPath, opts); err != nil {
			return err
		}
	}

	return nil
}

// ZipInfo contains information about a ZIP archive.
type ZipInfo struct {
	Path          string
	FileCount     int
	DirCount      int
	TotalSize     uint64
	RootDirectory string
}
