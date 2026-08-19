package worktree

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrNotADirectory indicates a copy source exists but is not a directory.
var ErrNotADirectory = errors.New("not a directory")

// ProgressFunc reports copy progress as files are written.
type ProgressFunc func(copied, total int)

// CopyTree recursively copies src to dst, preserving file modes.
//
// A missing source is not an error: a XenForo installation may legitimately
// have no data/ or internal_data/ yet.
//
// This is implemented natively rather than by shelling out to rsync. macOS no
// longer ships GNU rsync — /usr/bin/rsync is openrsync, which lacks the
// progress options — and Windows has no rsync at all, so an external tool would
// behave differently depending on the machine.
func CopyTree(ctx context.Context, src, dst string, progress ProgressFunc) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("failed to inspect %s: %w", src, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory: %w", src, ErrNotADirectory)
	}

	// filepath.Walk does not follow a symlink at the root of the walk, even
	// though os.Stat above does. Without this, a symlinked src is walked as
	// the link itself rather than the directory it points to.
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

	// Directory modes are applied after the walk, deepest first. Applying a
	// source mode on arrival would make a read-only directory (0555, say)
	// read-only before its own children were written, failing the copy.
	type pendingMode struct {
		path string
		mode os.FileMode
	}

	var pending []pendingMode

	walkErr := filepath.Walk(root, func(path string, entry os.FileInfo, walkErr error) error {
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
			// Created writable so the copy can proceed; the source mode is
			// applied once the directory's contents are in place.
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}

			pending = append(pending, pendingMode{path: target, mode: entry.Mode().Perm()})

			return nil

		case entry.Mode()&os.ModeSymlink != 0:
			return copySymlink(path, target)

		case !entry.Mode().IsRegular():
			// Sockets and devices have no meaning in a copy.
			return nil
		}

		if err := copyFile(path, target, entry.Mode().Perm()); err != nil {
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

	// Deepest first, so applying a restrictive mode to a parent cannot block
	// the chmod of a directory inside it.
	slices.SortFunc(pending, func(a, b pendingMode) int {
		return strings.Count(b.path, string(os.PathSeparator)) - strings.Count(a.path, string(os.PathSeparator))
	})

	for _, dir := range pending {
		// MkdirAll only applies a mode to directories it creates, and the
		// umask can clear bits even then, so an existing destination (e.g.
		// re-cloning over a worktree) can otherwise keep a stale mode.
		if err := os.Chmod(dir.path, dir.mode); err != nil {
			return fmt.Errorf("failed to set the mode of %s: %w", dir.path, err)
		}
	}

	return nil
}

// countFiles counts regular files so progress can be reported as a fraction.
func countFiles(ctx context.Context, root string) (int, error) {
	count := 0

	err := filepath.Walk(root, func(_ string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if entry.Mode().IsRegular() {
			count++
		}

		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count files in %s: %w", root, err)
	}

	return count, nil
}

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

	// OpenFile only applies mode at creation, and the umask can clear bits
	// even then, so an existing destination file keeps its old mode unless
	// it is set explicitly.
	if err := os.Chmod(dst, mode); err != nil {
		return fmt.Errorf("failed to set mode on %s: %w", dst, err)
	}

	return nil
}

func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("failed to read link %s: %w", src, err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(dst), err)
	}

	// A pre-existing link would make Symlink fail.
	_ = os.Remove(dst)

	if err := os.Symlink(target, dst); err != nil {
		return fmt.Errorf("failed to create link %s: %w", dst, err)
	}

	return nil
}
