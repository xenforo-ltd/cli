package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/worktree"
)

// clonedDirectories are the directories copied from the source installation.
//
// data/ holds public attachments and assets; internal_data/ holds private
// attachments, which are the main reason to clone at all. Everything else a
// XenForo install needs is either tracked in git or regenerated.
var clonedDirectories = []string{"data", "internal_data"}

// cloneEnvironment reproduces a source installation in a new worktree: its
// database first, then its files.
//
// The database is dumped and imported rather than copied, because each instance
// has its own named volume that the target's containers already own.
func cloneEnvironment(ctx context.Context, sourcePath, worktreePath string) error {
	sourceRunner, err := dockercompose.NewRunner(sourcePath)
	if err != nil {
		return fmt.Errorf("cannot clone from %s: %w", sourcePath, err)
	}

	targetRunner, err := dockercompose.NewRunner(worktreePath)
	if err != nil {
		return fmt.Errorf("cannot clone into %s: %w", worktreePath, err)
	}

	if err := cloneDatabase(ctx, sourceRunner, targetRunner); err != nil {
		return err
	}

	return cloneFiles(ctx, sourcePath, worktreePath)
}

// cloneDatabase streams a dump from the source instance into the target's.
func cloneDatabase(ctx context.Context, source, target *dockercompose.Runner) error {
	user, password := source.DatabaseCredentials()
	database := source.DatabaseName()

	dumpPath := filepath.Join(os.TempDir(), "xf-clone-"+target.Instance()+".sql")

	defer func() {
		_ = os.Remove(dumpPath)
	}()

	spinner := ui.NewSpinner("Exporting database from source...")
	spinner.Start()

	dump, err := os.Create(dumpPath)
	if err != nil {
		spinner.StopWithMessage("error", "Failed to export database")

		return fmt.Errorf("failed to create dump file: %w", err)
	}

	// --single-transaction keeps the source usable during the dump.
	dumpCmd := []string{
		"mariadb-dump",
		"--user=" + user,
		"--password=" + password,
		"--single-transaction",
		"--routines",
		"--events",
		database,
	}

	if err := source.ExecCapture(ctx, "mysql", dump, dumpCmd...); err != nil {
		_ = dump.Close()
		spinner.StopWithMessage("error", "Failed to export database")

		return fmt.Errorf("failed to export the source database: %w", err)
	}

	if err := dump.Close(); err != nil {
		spinner.StopWithMessage("error", "Failed to export database")

		return fmt.Errorf("failed to finish the dump: %w", err)
	}

	info, err := os.Stat(dumpPath)
	if err != nil {
		spinner.StopWithMessage("error", "Failed to export database")

		return fmt.Errorf("failed to inspect the dump: %w", err)
	}

	spinner.StopWithMessage("success", "Database exported ("+ui.FormatBytes(info.Size())+")")

	spinner = ui.NewSpinner("Importing database into worktree...")
	spinner.Start()

	restore, err := os.Open(dumpPath)
	if err != nil {
		spinner.StopWithMessage("error", "Failed to import database")

		return fmt.Errorf("failed to read the dump: %w", err)
	}

	defer func() {
		_ = restore.Close()
	}()

	targetUser, targetPassword := target.DatabaseCredentials()

	importCmd := []string{
		"mariadb",
		"--user=" + targetUser,
		"--password=" + targetPassword,
		target.DatabaseName(),
	}

	if err := target.ExecInput(ctx, "mysql", restore, importCmd...); err != nil {
		spinner.StopWithMessage("error", "Failed to import database")

		return fmt.Errorf("failed to import the database: %w", err)
	}

	spinner.StopWithMessage("success", "Database imported")

	return nil
}

// cloneFiles copies the source's user content into the worktree.
func cloneFiles(ctx context.Context, sourcePath, worktreePath string) error {
	for _, dir := range clonedDirectories {
		src := filepath.Join(sourcePath, dir)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		spinner := ui.NewSpinner("Copying " + dir + "...")
		spinner.Start()

		var lastReported int

		err := worktree.CopyTree(ctx, src, filepath.Join(worktreePath, dir), func(copied, total int) {
			// Updating on every file would spend more time rendering than
			// copying, since code_cache alone is thousands of small files.
			if total > 0 && (copied == total || copied-lastReported >= progressUpdateInterval) {
				lastReported = copied

				spinner.UpdateMessage(fmt.Sprintf("Copying %s... %d/%d files", dir, copied, total))
			}
		})
		if err != nil {
			spinner.StopWithMessage("error", "Failed to copy "+dir)

			return fmt.Errorf("failed to copy %s: %w", dir, err)
		}

		spinner.StopWithMessage("success", "Copied "+dir)
	}

	return nil
}

// progressUpdateInterval is how many files to copy between progress updates.
const progressUpdateInterval = 100
