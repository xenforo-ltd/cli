// Package extract provides utilities for extracting zip files.
package extract

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xenforo-ltd/cli/internal/clierrors"
)

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
		return clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to open zip file", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return clierrors.Wrap(clierrors.CodeDirCreateFailed, "failed to create destination directory", err)
	}

	total := len(reader.File)
	current := 0

	for _, file := range reader.File {
		current++

		if isSymlink(file) {
			return clierrors.Newf(clierrors.CodeValidationFailed, "symlink entries are not allowed in archive: %s", file.Name)
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
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return clierrors.Wrapf(clierrors.CodeDirCreateFailed, err, "failed to create directory: %s", name)
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
	if isSymlink(file) {
		return clierrors.Newf(clierrors.CodeValidationFailed, "symlink entries are not allowed in archive: %s", file.Name)
	}
	if !opts.OverwriteExisting {
		if _, err := os.Stat(destPath); err == nil {
			return nil
		}
	}

	parentDir := filepath.Dir(destPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return clierrors.Wrapf(clierrors.CodeDirCreateFailed, err, "failed to create directory: %s", parentDir)
	}

	srcFile, err := file.Open()
	if err != nil {
		return clierrors.Wrapf(clierrors.CodeFileReadFailed, err, "failed to open file in archive: %s", file.Name)
	}
	defer srcFile.Close()

	mode := file.Mode()
	if !opts.PreservePermissions {
		mode = 0644
	}

	destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return clierrors.Wrapf(clierrors.CodeFileWriteFailed, err, "failed to create file: %s", destPath)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return clierrors.Wrapf(clierrors.CodeFileWriteFailed, err, "failed to write file: %s", destPath)
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
		return "", clierrors.Newf(clierrors.CodeValidationFailed, "invalid path in archive: %s", name)
	}
	if strings.Contains(name, "\\") {
		return "", clierrors.Newf(clierrors.CodeValidationFailed, "invalid path in archive: %s", name)
	}
	if len(name) >= 2 {
		c := name[0]
		if name[1] == ':' && ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return "", clierrors.Newf(clierrors.CodeValidationFailed, "invalid path in archive: %s", name)
		}
	}
	if filepath.IsAbs(name) {
		return "", clierrors.Newf(clierrors.CodeValidationFailed, "invalid path in archive: %s", name)
	}

	name = filepath.Clean(name)

	destPath := filepath.Join(destDir, name)

	cleanDestDir := filepath.Clean(destDir)
	cleanDestPath := filepath.Clean(destPath)

	rel, err := filepath.Rel(cleanDestDir, cleanDestPath)
	if err != nil {
		return "", clierrors.Wrap(clierrors.CodeValidationFailed, "invalid path in archive", err)
	}
	if rel == "." {
		return cleanDestPath, nil
	}
	if strings.HasPrefix(rel, "..") || rel == "" {
		return "", clierrors.Newf(clierrors.CodeValidationFailed, "invalid path in archive: %s", name)
	}

	return destPath, nil
}

func isSymlink(file *zip.File) bool {
	return file.Mode()&os.ModeSymlink != 0
}

// ListZipContents lists the contents of a zip file.
func ListZipContents(zipPath string) ([]string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to open zip file", err)
	}
	defer reader.Close()

	var files []string
	for _, file := range reader.File {
		files = append(files, file.Name)
	}

	return files, nil
}

// GetZipRootDirectory returns the common root directory of all files in the ZIP.
// XenForo ZIPs typically have all files under an "upload/" directory.
func GetZipRootDirectory(zipPath string) (string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to open zip file", err)
	}
	defer reader.Close()

	if len(reader.File) == 0 {
		return "", clierrors.New(clierrors.CodeValidationFailed, "zip file is empty")
	}

	firstFile := reader.File[0].Name
	parts := strings.SplitN(filepath.ToSlash(firstFile), "/", 2)
	if len(parts) == 0 {
		return "", nil
	}

	root := parts[0]

	for _, file := range reader.File {
		fileParts := strings.SplitN(filepath.ToSlash(file.Name), "/", 2)
		if len(fileParts) == 0 || fileParts[0] != root {
			return "", nil
		}
	}

	return root, nil
}

// ExtractXenForoZip extracts a XenForo ZIP file to the destination.
// It extracts only files from within the "upload/" directory, stripping that prefix.
// This handles XenForo's ZIP structure where files are under upload/ but there may
// be other files like README at the root.
func ExtractXenForoZip(zipPath, destDir string, onProgress func(current, total int, filename string)) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to open zip file", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return clierrors.Wrap(clierrors.CodeDirCreateFailed, "failed to create destination directory", err)
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
			return clierrors.Newf(clierrors.CodeValidationFailed, "symlink entries are not allowed in archive: %s", file.Name)
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
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return clierrors.Wrapf(clierrors.CodeDirCreateFailed, err, "failed to create directory: %s", name)
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

// CountZipFiles counts the number of files in a zip archive.
func CountZipFiles(zipPath string) (int, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to open zip file", err)
	}
	defer reader.Close()

	count := 0
	for _, file := range reader.File {
		if !file.FileInfo().IsDir() {
			count++
		}
	}

	return count, nil
}

// ZipInfo contains information about a ZIP archive.
type ZipInfo struct {
	Path          string
	FileCount     int
	DirCount      int
	TotalSize     uint64
	RootDirectory string
}

// GetZipInfo returns metadata about a zip file.
func GetZipInfo(zipPath string) (*ZipInfo, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, clierrors.Wrap(clierrors.CodeFileReadFailed, "failed to open zip file", err)
	}
	defer reader.Close()

	info := &ZipInfo{
		Path: zipPath,
	}

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			info.DirCount++
		} else {
			info.FileCount++
			info.TotalSize += file.UncompressedSize64
		}
	}

	root, _ := GetZipRootDirectory(zipPath)
	info.RootDirectory = root

	return info, nil
}
