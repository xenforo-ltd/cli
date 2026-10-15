package build

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// zipTask writes a release archive and records its file count and uncompressed
// size, excluding the generated hash manifest.
type zipTask struct {
	selection  uploadSelection
	upload     map[string]string
	hashesPath string
	zipPath    string
	timestamp  time.Time
	fileCount  int
	totalSize  int64
}

// run writes upload files and the hash manifest beneath "upload/", with
// _no_upload contents at the archive root.
func (t *zipTask) run(ctx context.Context, root *os.Root) error {
	collected, err := t.selection.collectNoUpload(ctx, root)
	if err != nil {
		return err
	}

	files := make(map[string]string, len(t.upload)+1)
	for rel, source := range t.upload {
		files[path.Join("upload", rel)] = source
	}

	manifest := path.Join("upload", t.selection.prefix, t.selection.addOnDir, "hashes.json")
	files[manifest] = t.hashesPath
	for _, f := range collected {
		rel, _ := within(f.path, path.Join(t.selection.addOnDir, "_no_upload"))

		if f.err != nil {
			return fmt.Errorf("reading _no_upload: %w", f.err)
		}

		if rel == "." && !f.isDir() {
			return fmt.Errorf("_no_upload must be a directory")
		}

		if f.isDir() {
			continue
		}

		if err := f.regular(); err != nil {
			return err
		}

		if _, exists := files[rel]; exists {
			return fmt.Errorf("_no_upload path %q conflicts with upload content", rel)
		}

		files[rel] = f.path
	}

	keys := slices.Sorted(maps.Keys(files))
	for _, key := range keys {
		if !fs.ValidPath(key) || strings.ContainsAny(key, `\:`) {
			return fmt.Errorf("invalid archive path %q", key)
		}
	}

	var count int
	var total int64
	err = atomicWrite(ctx, root, t.zipPath, func(out *os.File) error {
		output := bufio.NewWriterSize(out, ioBufferSize)
		zw := zip.NewWriter(output)
		sources := directoryOpener{root: root}
		defer sources.Close()
		buffer := make([]byte, 32*1024)
		for _, rel := range keys {
			size, err := addFileToArchive(ctx, &sources, zw, rel, files[rel], t.timestamp, buffer)
			if err != nil {
				_ = zw.Close()
				return err
			}

			if rel != manifest {
				count++
				total += size
			}
		}

		if err := zw.Close(); err != nil {
			return err
		}
		return output.Flush()
	})

	if err != nil {
		return fmt.Errorf("creating release ZIP: %w", err)
	}

	t.fileCount, t.totalSize = count, total
	return nil
}

// collectNoUpload discovers implicit archive extras independently of upload
// filters. Hidden paths still require an explicit whitelist pattern.
func (s uploadSelection) collectNoUpload(ctx context.Context, root *os.Root) ([]file, error) {
	base := path.Join(s.addOnDir, "_no_upload")
	if _, err := root.Lstat(filepath.FromSlash(base)); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("reading _no_upload: %w", err)
	}
	return collectFilesWhere(ctx, root, base, func(name string, isDir bool) bool {
		rel, _ := within(name, base)
		return !isHiddenPath(rel) || s.matchesWhitelist(name, isDir)
	})
}

// addFileToArchive adds source as name with normalized permissions and the given
// timestamp, and returns its uncompressed size.
func addFileToArchive(ctx context.Context, root fileSource, zw *zip.Writer, name, source string, timestamp time.Time, buffer []byte) (int64, error) {
	f, info, err := openRegular(ctx, root, source)
	if err != nil {
		return 0, fmt.Errorf("opening %s: %w", source, err)
	}

	defer f.Close()

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return 0, fmt.Errorf("file header for %s: %w", source, err)
	}

	header.Name, header.Method = name, zip.Deflate
	header.Modified = timestamp
	mode := fs.FileMode(0o644)
	if info.Mode()&0o111 != 0 {
		mode = 0o755
	}

	header.SetMode(mode)
	w, err := zw.CreateHeader(header)
	if err != nil {
		return 0, fmt.Errorf("creating ZIP entry %s: %w", name, err)
	}

	// Hide File.WriteTo so CopyBuffer uses the archive's shared buffer.
	n, err := io.CopyBuffer(w, struct{ io.Reader }{f}, buffer)
	if err != nil {
		return 0, fmt.Errorf("writing ZIP entry %s: %w", name, err)
	}

	return n, nil
}
