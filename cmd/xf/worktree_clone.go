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
//
// The whole operation narrates through a single spinner, so success or failure
// is reported exactly once.
func cloneEnvironment(ctx context.Context, sourcePath, worktreePath string) error {
	sourceRunner, err := dockercompose.NewRunner(sourcePath)
	if err != nil {
		return fmt.Errorf("cannot clone from %s: %w", sourcePath, err)
	}

	targetRunner, err := dockercompose.NewRunner(worktreePath)
	if err != nil {
		return fmt.Errorf("cannot clone into %s: %w", worktreePath, err)
	}

	spinner := ui.NewSpinner("Exporting database")
	spinner.Start()

	dbSize, err := cloneDatabase(ctx, spinner, sourceRunner, targetRunner)
	if err != nil {
		spinner.Stop()

		return err
	}

	copiedDirs, err := cloneFiles(ctx, spinner, sourcePath, worktreePath)
	if err != nil {
		spinner.Stop()

		return err
	}

	if err := retargetBoardIdentity(ctx, spinner, targetRunner, filepath.Base(worktreePath)); err != nil {
		spinner.Stop()

		return err
	}

	message := fmt.Sprintf("Environment cloned (database %s)", ui.FormatBytes(dbSize))
	if len(copiedDirs) > 0 {
		message = fmt.Sprintf("Environment cloned (database %s, %s)", ui.FormatBytes(dbSize), strings.Join(copiedDirs, ", "))
	}

	spinner.StopWithMessage("success", message)

	return nil
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
func retargetBoardIdentity(ctx context.Context, spinner *ui.Spinner, target *dockercompose.Runner, label string) error {
	url, err := target.GetURL(ctx)
	if err != nil || url == "" {
		spinner.Stop()
		ui.PrintWarning("Could not determine the worktree's URL; the board URL still points at the source")

		return nil
	}

	spinner.UpdateMessage("Updating board URL and title")

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
		ui.PrintWarning("Could not update the board URL to " + url)
		ui.PrintHint("Set it in the admin control panel under Options > Basic board information")

		return nil
	}

	return nil
}

// phpQuote renders a string as a single-quoted PHP literal.
func phpQuote(value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")

	return "'" + escaped + "'"
}

// cloneDatabase streams a dump from the source instance into the target's,
// narrating progress through spinner. It returns the size of the dump.
func cloneDatabase(ctx context.Context, spinner *ui.Spinner, source, target *dockercompose.Runner) (int64, error) {
	user, password := source.DatabaseCredentials()
	database := source.DatabaseName()

	// CreateTemp generates an unpredictable name and creates the file mode
	// 0600. A fixed path in the shared temp directory would leave the whole
	// forum database, including password hashes, readable by other users on
	// the host, and would let them pre-create the path as a symlink.
	dump, err := os.CreateTemp("", "xf-clone-"+target.Instance()+"-*.sql")
	if err != nil {
		return 0, fmt.Errorf("failed to export the database: %w", err)
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

		return 0, fmt.Errorf("failed to export the database: %w", err)
	}

	if err := dump.Close(); err != nil {
		return 0, fmt.Errorf("failed to export the database: %w", err)
	}

	info, err := os.Stat(dumpPath)
	if err != nil {
		return 0, fmt.Errorf("failed to export the database: %w", err)
	}

	spinner.UpdateMessage("Importing database")

	restore, err := os.Open(dumpPath)
	if err != nil {
		return 0, fmt.Errorf("failed to import the database: %w", err)
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
		return 0, fmt.Errorf("failed to import the database: %w", err)
	}

	return info.Size(), nil
}

// cloneFiles copies the source's user content into the worktree, narrating
// progress through spinner. It returns the directories that were copied.
func cloneFiles(ctx context.Context, spinner *ui.Spinner, sourcePath, worktreePath string) ([]string, error) {
	copied := make([]string, 0, len(clonedDirectories))

	for _, dir := range clonedDirectories {
		src := filepath.Join(sourcePath, dir)

		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}

		spinner.UpdateMessage(fmt.Sprintf("Copying %s", dir))

		var lastReported int

		err := worktree.CopyTree(ctx, src, filepath.Join(worktreePath, dir), func(done, total int) {
			// Updating on every file would spend more time rendering than
			// copying, since code_cache alone is thousands of small files.
			if total > 0 && (done == total || done-lastReported >= progressUpdateInterval) {
				lastReported = done

				spinner.UpdateMessage(fmt.Sprintf("Copying %s (%d/%d files)", dir, done, total))
			}
		})
		if err != nil {
			return nil, fmt.Errorf("failed to copy %s: %w", dir, err)
		}

		copied = append(copied, dir)
	}

	return copied, nil
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
