package worktree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// errNotADirectory indicates a copy source exists but is not a directory.
var errNotADirectory = errors.New("not a directory")

// progressFunc reports copy progress as files are written.
type progressFunc func(copied, total int)

// copyTree recursively copies src to dst, preserving file modes.
//
// A missing source is not an error: a XenForo installation may legitimately
// have no data/ or internal_data/ yet.
// A symlinked source root is resolved; links within the tree are copied as
// links and special files are skipped. Directory modes are applied deepest
// first after copying, so restrictive permissions do not prevent writing children.
//
// This is implemented natively rather than by shelling out to rsync. macOS no
// longer ships GNU rsync — /usr/bin/rsync is openrsync, which lacks the
// progress options — and Windows has no rsync at all, so an external tool would
// behave differently depending on the machine.
func copyTree(ctx context.Context, src, dst string, progress progressFunc) error {
	info, err := os.Stat(src)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to inspect %s: %w", src, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %w", src, errNotADirectory)
	}

	root, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", src, err)
	}

	total := 0

	if progress != nil {
		total, err = countFiles(ctx, root)
		if err != nil {
			return err
		}
	}

	copied := 0

	type pendingMode struct {
		path string
		mode os.FileMode
	}

	var pending []pendingMode

	walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("failed to resolve %s: %w", path, err)
		}

		target := filepath.Join(dst, rel)

		switch {
		case entry.IsDir():
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}

			info, err := entry.Info()
			if err != nil {
				return err
			}

			pending = append(pending, pendingMode{path: target, mode: info.Mode().Perm()})

			return nil

		case entry.Type()&os.ModeSymlink != 0:
			return copySymlink(path, target)

		case !entry.Type().IsRegular():
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		if err := copyFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}

		copied++

		if progress != nil {
			progress(copied, total)
		}

		return nil
	})

	if walkErr != nil {
		return walkErr
	}

	slices.SortFunc(pending, func(a, b pendingMode) int {
		return strings.Count(b.path, string(os.PathSeparator)) - strings.Count(a.path, string(os.PathSeparator))
	})

	for _, dir := range pending {
		if err := os.Chmod(dir.path, dir.mode); err != nil {
			return fmt.Errorf("failed to set the mode of %s: %w", dir.path, err)
		}
	}

	return nil
}

// countFiles counts regular files so progress can be reported as a fraction.
func countFiles(ctx context.Context, root string) (int, error) {
	count := 0

	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if entry.Type().IsRegular() {
			count++
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to count files in %s: %w", root, err)
	}

	return count, nil
}

// copyFile copies a regular file and applies its mode even when umask would
// restrict it. It attempts to remove the destination first to avoid following
// a symlink and truncating its target, which may be in the source tree.
func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(dst), err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", src, err)
	}

	defer func() {
		_ = in.Close()
	}()

	_ = os.Remove(dst)

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", dst, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()

		return fmt.Errorf("failed to copy %s: %w", src, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", dst, err)
	}

	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("failed to set mode on %s: %w", dst, err)
	}

	return nil
}

// copySymlink replaces dst with a link to the same target as src. Absolute
// targets are preserved, including links back into the source tree.
func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("failed to read link %s: %w", src, err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(dst), err)
	}

	_ = os.Remove(dst)

	if err := os.Symlink(target, dst); err != nil {
		return fmt.Errorf("failed to create link %s: %w", dst, err)
	}

	return nil
}
