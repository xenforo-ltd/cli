package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

	if err := cloneFiles(ctx, sourcePath, worktreePath); err != nil {
		return err
	}

	return retargetBoardIdentity(ctx, targetRunner, filepath.Base(worktreePath))
}

// retargetBoardIdentity points the cloned board at its own address and marks
// its title with the worktree name.
//
// boardUrl is stored in the database, so a clone inherits the source's URL and
// generates links back to the forum it was copied from. XenForo has no config
// override for options, so the value must be updated in the database.
//
// OptionRepository::updateOptions is used rather than a direct UPDATE because
// it also rebuilds the option cache. Writing the row alone would leave the old
// URL in service until something else happened to rebuild it.
func retargetBoardIdentity(ctx context.Context, target *dockercompose.Runner, label string) error {
	url, err := target.GetURL(ctx)
	if err != nil || url == "" {
		ui.PrintWarning("Could not determine the worktree's URL; the board URL still points at the source")

		return nil
	}

	spinner := ui.NewSpinner("Updating board URL and title...")
	spinner.Start()

	// Run through XenForo's own bootstrap so the repository and cache rebuild
	// behave exactly as they do for xf:install.
	// php -r takes bare statements, without an opening tag.
	// Both options are set in one call so the option cache rebuilds once. The
	// title is derived inside PHP because it depends on the current value,
	// which is only known once XenForo has booted.
	script := fmt.Sprintf(
		`require __DIR__ . '/src/XF.php';`+
			`XF::start(__DIR__);`+
			`$app = XF::setupApp(XF\App::class);`+
			`$title = rtrim(preg_replace('/\s*\[[^\[\]]*\]\s*$/', '', $app->options()->boardTitle));`+
			`$title = $title === '' ? %[2]s : $title . ' ' . %[2]s;`+
			`$app->repository(XF\Repository\OptionRepository::class)`+
			`->updateOptions(['boardUrl' => %[1]s, 'boardTitle' => $title]);`,
		phpQuote(url),
		phpQuote("["+label+"]"),
	)

	if err := target.PHP(ctx, "-r", script); err != nil {
		spinner.Stop()
		ui.PrintWarning(fmt.Sprintf("Could not update the board URL to %s: %v", url, err))
		ui.Println("    Set it in the admin control panel under Options > Basic board information.")

		return nil
	}

	spinner.StopWithMessage("success", "Board URL set to "+url)

	return nil
}

// phpQuote renders a string as a single-quoted PHP literal.
func phpQuote(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")

	return "'" + escaped + "'"
}

// cloneDatabase streams a dump from the source instance into the target's.
func cloneDatabase(ctx context.Context, source, target *dockercompose.Runner) error {
	user, password := source.DatabaseCredentials()
	database := source.DatabaseName()

	spinner := ui.NewSpinner("Exporting database from source...")
	spinner.Start()

	// CreateTemp generates an unpredictable name and creates the file mode
	// 0600. A fixed path in the shared temp directory would leave the whole
	// forum database, including password hashes, readable by other users on
	// the host, and would let them pre-create the path as a symlink.
	dump, err := os.CreateTemp("", "xf-clone-"+target.Instance()+"-*.sql")
	if err != nil {
		spinner.StopWithMessage("error", "Failed to export database")

		return fmt.Errorf("failed to create dump file: %w", err)
	}

	dumpPath := dump.Name()

	defer func() {
		_ = os.Remove(dumpPath)
	}()

	// The password goes in the environment: an argument would be visible to
	// anything that can list processes in the container.
	dumpEnv := map[string]string{"MYSQL_PWD": password}

	// --single-transaction keeps the source usable during the dump.
	dumpCmd := []string{
		"mariadb-dump",
		"--user=" + user,
		"--single-transaction",
		"--routines",
		"--events",
		database,
	}

	if err := source.ExecCaptureWithEnv(ctx, "mysql", dumpEnv, dump, dumpCmd...); err != nil {
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

	importEnv := map[string]string{"MYSQL_PWD": targetPassword}

	importCmd := []string{
		"mariadb",
		"--user=" + targetUser,
		target.DatabaseName(),
	}

	if err := target.ExecInputWithEnv(ctx, "mysql", importEnv, restore, importCmd...); err != nil {
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

// retitleBoard appends a worktree label to a board title, replacing any label
// already present.
//
// A clone inherits the source forum's title, so several worktrees would
// otherwise be indistinguishable in a browser tab.
//
// This mirrors the expression used in retargetBoardIdentity, which has to run
// inside PHP because it depends on the live option value. It exists separately
// so the behaviour can be tested directly.
func retitleBoard(title, label string) string {
	trimmed := strings.TrimRight(trailingLabel.ReplaceAllString(title, ""), " \t")

	suffix := "[" + label + "]"

	if trimmed == "" {
		return suffix
	}

	return trimmed + " " + suffix
}

// trailingLabel matches a bracketed label at the end of a board title. Nested
// brackets are excluded so a title ending in "[a [b]]" is left alone rather
// than partly consumed.
var trailingLabel = regexp.MustCompile(`\s*\[[^\[\]]*\]\s*$`)
