package build

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// ioBufferSize bounds reusable hash-input and ZIP-output buffers.
const ioBufferSize = 64 * 1024

// file records a build-root-relative path and the result of inspecting it.
type file struct {
	path string
	info fs.FileInfo
	err  error
}

// isDir reports whether f has directory metadata.
func (f file) isDir() bool { return f.info != nil && f.info.IsDir() }

// regular returns an error if f could not be inspected or is not a regular file.
func (f file) regular() error {
	if f.err != nil {
		return fmt.Errorf("reading %s: %w", f.path, f.err)
	}

	if f.info == nil || !f.info.Mode().IsRegular() {
		return fmt.Errorf("selected path %s is not a regular file", f.path)
	}

	return nil
}

// collectFiles returns root and its descendants in path order, following symlinks
// that remain within root. Entry errors are recorded in each file so selection
// can ignore unrelated paths. Cancellation stops collection.
func collectFiles(ctx context.Context, root *os.Root) ([]file, error) {
	var files []file
	var active []file
	var walk func(string) error
	walk = func(name string) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		info, err := root.Stat(filepath.FromSlash(name))
		if err != nil {
			files = append(files, file{path: name, err: err})
			return nil
		}

		for _, previous := range active {
			if os.SameFile(info, previous.info) {
				files = append(files, file{path: name, info: info, err: fmt.Errorf("symlink loop (already visiting %s)", previous.path)})
				return nil
			}
		}

		entries, err := fs.ReadDir(root.FS(), name)
		files = append(files, file{path: name, info: info, err: err})
		if err != nil {
			return nil
		}

		active = append(active, file{path: name, info: info})
		defer func() { active = active[:len(active)-1] }()
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}

			child := path.Join(name, entry.Name())
			info, err := root.Stat(filepath.FromSlash(child))
			if err == nil && info.IsDir() {
				if err := walk(child); err != nil {
					return err
				}
			} else {
				files = append(files, file{path: child, info: info, err: err})
			}
		}

		return nil
	}

	if err := walk("."); err != nil {
		return nil, err
	}

	slices.SortFunc(files, func(a, b file) int { return strings.Compare(a.path, b.path) })
	return files, ctx.Err()
}

// openInstallation resolves name to an absolute path and opens its symlink target.
// The caller must close the returned root.
func openInstallation(ctx context.Context, name string) (*os.Root, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	abs, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}

	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}

	return os.OpenRoot(real)
}

// readFile reads the contents of a regular file within root.
func readFile(ctx context.Context, root fileSource, name string) ([]byte, error) {
	f, info, err := openRegular(ctx, root, name)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	var buffer bytes.Buffer
	// ReadFrom needs MinRead bytes of spare capacity for its final EOF read.
	if size := info.Size(); size > 0 && size <= int64(math.MaxInt-bytes.MinRead) {
		buffer.Grow(int(size) + bytes.MinRead)
	}
	_, err = buffer.ReadFrom(f)
	return buffer.Bytes(), err
}

// writeFile replaces name with data using a temporary file.
// It creates parent directories as needed and sets the file mode to 0644.
func writeFile(ctx context.Context, root *os.Root, name string, data []byte) error {
	return atomicWrite(ctx, root, name, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	})
}

// atomicWrite calls write on a temporary file beside name, then closes and renames
// it over name. It checks for cancellation before publishing the file.
// write must not close the supplied file.
func atomicWrite(ctx context.Context, root *os.Root, name string, write func(*os.File) error) error {
	name, err := relativePath(name, literalPath)
	if err != nil {
		return err
	}

	if err := root.MkdirAll(filepath.FromSlash(path.Dir(name)), 0o755); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	temporary := filepath.FromSlash(path.Join(path.Dir(name), ".build-"+rand.Text()))
	out, err := root.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	defer root.Remove(temporary)
	defer out.Close()
	if err := write(out); err != nil {
		return err
	}

	if err := out.Chmod(0o644); err != nil {
		return err
	}

	if err := out.Close(); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return root.Rename(temporary, filepath.FromSlash(name))
}

// openRegular opens a regular file for reading within root and returns its metadata.
// name must be a slash-separated relative path. The caller must close the file.
func openRegular(ctx context.Context, root fileSource, name string) (*os.File, fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	native, err := filepath.Localize(name)
	if err != nil {
		return nil, nil, err
	}

	f, err := root.Open(native)
	if err != nil {
		return nil, nil, err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}

	if !info.Mode().IsRegular() {
		f.Close()
		return nil, nil, fmt.Errorf("selected path %s is not a regular file", name)
	}

	return f, info, nil
}
