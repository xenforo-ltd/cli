package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/worktree"
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Create and manage development worktrees",
	Long: `Create a git worktree with a fully configured XenForo environment.

A worktree is a second checkout of the same repository on its own branch, with
its own Docker containers and database. It lets you work on a feature without
disturbing your main checkout.

Worktrees are created alongside the source checkout, so ~/Sites/main gains
~/Sites/main.worktrees/<branch>. The path is derived from the branch name and is
always predictable.

'xf worktree create <branch>' creates the worktree and then initialises the
environment: Docker configuration, containers, Composer dependencies and the
XenForo installation.

Examples:
  # Create a worktree and set up its environment
  xf worktree create dev/24x/feature

  # Branch from somewhere other than the current HEAD
  xf worktree create dev/24x/feature --base main

  # Create the worktree without setting anything up
  xf worktree create dev/24x/feature --no-setup

  # Print the path of an existing worktree
  cd "$(xf worktree path dev/24x/feature)"`,
	// This command only dispatches subcommands. Taking a branch here too would
	// make `xf worktree lst` ambiguous, and cobra would resolve it by silently
	// creating a branch called "lst" rather than reporting a mistyped
	// subcommand.
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}

		// `xf worktree help` reads as a request for help. Cobra reserves "help"
		// at the root only, so it arrives here as an unknown subcommand.
		if args[0] == "help" {
			return nil
		}

		return fmt.Errorf("unknown command %q for %q: %w", args[0], cmd.CommandPath(), ErrInvalidInput)
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var worktreeCreateCmd = &cobra.Command{
	Use:   "create <branch>",
	Short: "Create a worktree and set up its environment",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runWorktreeCreate,
}

var worktreeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List worktrees for this project",
	Args:  cobra.NoArgs,
	RunE:  runWorktreeList,
}

var worktreeListAllCmd = &cobra.Command{
	Use:   "list-all",
	Short: "List worktrees across all known projects",
	Args:  cobra.NoArgs,
	RunE:  runWorktreeListAll,
}

var worktreePathCmd = &cobra.Command{
	Use:   "path <branch>",
	Short: "Print the path of a worktree",
	Long: `Print the resolved path for a branch's worktree.

The path is derived from the branch name, so this works whether or not the
worktree exists. Useful for shell and agent use:

  cd "$(xf worktree path dev/24x/feature)"

Branch names that resolve to no directory of their own, such as "." or "..",
are rejected rather than printing the directory that holds every worktree.`,
	Args: cobra.ExactArgs(1),
	RunE: runWorktreePath,
}

var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <branch>",
	Short: "Remove a worktree and its containers",
	Long: `Remove a worktree, its branch, and its Docker containers and volumes.

Refuses when the worktree contains uncommitted changes or commits that exist on
no remote, listing what would be lost. Use --force to remove it anyway.`,
	Args: cobra.ExactArgs(1),
	RunE: runWorktreeRemove,
}

var worktreePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Drop registry entries for worktrees that no longer exist",
	Args:  cobra.NoArgs,
	RunE:  runWorktreePrune,
}

// Defaults for the throwaway installation a worktree gets.
const (
	defaultWorktreeAdminUser     = "admin"
	defaultWorktreeAdminPassword = "password"
	defaultWorktreeAdminEmail    = "admin@example.com"
)

var (
	flagWorktreeBase           string
	flagWorktreeAdminUser      string
	flagWorktreeAdminPassword  string
	flagWorktreeAdminEmail     string
	flagWorktreeTitle          string
	flagWorktreeNoSetup        bool
	flagWorktreeNoUp           bool
	flagWorktreeInstance       string
	flagWorktreeJSON           bool
	flagWorktreeForce          bool
	flagWorktreeKeepContainers bool
	flagWorktreeFresh          bool
)

func init() {
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeBase, "base", "", "ref to branch from (defaults to current HEAD)")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeNoSetup, "no-setup", false, "create the worktree only, without setting up the environment")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeNoUp, "no-up", false, "configure the environment but do not start containers")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeFresh, "fresh", false, "install a clean forum instead of cloning the source environment")
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeInstance, "instance", "", "Docker instance name")
	// Known limitation: setup progress from init and cloning still goes to
	// stdout, so the stream is only pure JSON when --no-setup is used. The
	// output layer writes through package-level helpers with no injectable
	// writer, so routing it to stderr is a wider change than this command.
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "output as JSON (setup progress is still written to stdout)")
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeAdminUser, "admin-user", "", "admin username (default \"admin\")")
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeAdminPassword, "admin-password", "", "admin password (default \"password\")")
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeAdminEmail, "admin-email", "", "admin email (default \"admin@example.com\")")
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeTitle, "title", "", "site title (defaults to the branch name)")

	worktreeListCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "output as JSON")
	worktreeListAllCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "output as JSON")
	worktreeRemoveCmd.Flags().BoolVar(&flagWorktreeForce, "force", false, "remove even if there are uncommitted changes or unpushed commits")
	worktreeRemoveCmd.Flags().BoolVar(&flagWorktreeKeepContainers, "keep-containers", false, "leave the Docker containers and volumes in place")

	worktreeCmd.AddCommand(worktreeCreateCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeListAllCmd)
	worktreeCmd.AddCommand(worktreePathCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)
	worktreeCmd.AddCommand(worktreePruneCmd)

	rootCmd.AddCommand(worktreeCmd)
}

// worktreeOutput is the machine-readable form of a created worktree.
type worktreeOutput struct {
	Path         string    `json:"path"`
	Branch       string    `json:"branch"`
	SourcePath   string    `json:"source_path"`
	SourceBranch string    `json:"source_branch"`
	Instance     string    `json:"instance"`
	Cloned       bool      `json:"cloned"`
	CreatedAt    time.Time `json:"created_at"`
}

func runWorktreeCreate(cmd *cobra.Command, args []string) error {
	branch, err := resolveBranchArg(args)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	result, err := worktree.Create(cmd.Context(), worktree.Options{
		SourcePath: cwd,
		Branch:     branch,
		Base:       flagWorktreeBase,
		Instance:   flagWorktreeInstance,
	})
	if err != nil {
		return err
	}

	entry := worktree.Entry{
		SourcePath:   result.SourcePath,
		SourceBranch: result.SourceBranch,
		WorktreePath: result.Path,
		Branch:       result.Branch,
		Instance:     result.Instance,
		CreatedAt:    result.CreatedAt,
	}

	if err := recordWorktree(entry); err != nil {
		// The worktree exists and is usable; a registry failure must not
		// present itself as a failed creation.
		ui.PrintWarning(fmt.Sprintf("Could not record worktree in the registry: %v", err))
	}

	if !flagWorktreeJSON {
		ui.PrintSuccess("Created worktree " + result.Path)
		ui.PrintKeyValuePadded([]ui.KVPair{
			ui.KV("Branch", result.Branch),
			ui.KV("Based on", result.SourceBranch),
			ui.KV("Instance", result.Instance),
		})
	}

	// Cloning imports a database that is already installed, so xf:install must
	// not run over it: it would wipe the data that was just copied.
	//
	// Cloning needs running containers, so --no-up rules it out. Treating the
	// worktree as cloning anyway would suppress xf:install as well, leaving it
	// with neither an imported database nor an installed one.
	cloning := false

	if !flagWorktreeFresh && !flagWorktreeNoUp {
		installed, err := sourceIsInstalled(result.SourcePath)
		if err != nil {
			return err
		}

		cloning = installed
	}

	if !flagWorktreeNoSetup {
		if err := setUpWorktree(cmd.Context(), result, worktreeInitOptions(result, cloning)); err != nil {
			return err
		}

		if cloning {
			if err := cloneEnvironment(cmd.Context(), result.SourcePath, result.Path); err != nil {
				return fmt.Errorf("worktree created at %s, but cloning the environment failed: %w", result.Path, err)
			}

			entry.Cloned = true

			if err := recordWorktree(entry); err != nil {
				ui.PrintWarning(fmt.Sprintf("Could not record worktree in the registry: %v", err))
			}
		}

		// Only a fresh install has credentials worth reporting. A cloned
		// worktree keeps the source's own logins, so printing the defaults
		// would be wrong.
		if !cloning && !flagWorktreeJSON && !flagWorktreeNoUp {
			ui.Println()
			ui.PrintKeyValuePadded([]ui.KVPair{
				ui.KV("Admin user", defaultString(flagWorktreeAdminUser, defaultWorktreeAdminUser)),
				ui.KV("Admin password", defaultString(flagWorktreeAdminPassword, defaultWorktreeAdminPassword)),
			})
		}
	}

	if flagWorktreeJSON {
		return printJSON(worktreeOutput{
			Path:         result.Path,
			Branch:       result.Branch,
			SourcePath:   result.SourcePath,
			SourceBranch: result.SourceBranch,
			Instance:     result.Instance,
			Cloned:       entry.Cloned,
			CreatedAt:    result.CreatedAt,
		})
	}

	return nil
}

// worktreeInitOptions builds the init options for a new worktree.
//
// cloning reports whether the source environment will be copied in, which
// suppresses xf:install: the imported database is already installed, and
// reinstalling would wipe the data that was just copied.
func worktreeInitOptions(result *worktree.Result, cloning bool) *InitOptions {
	// A worktree is a disposable development environment, so a fresh install
	// uses fixed credentials rather than prompting. Knowing the login without
	// being asked is the point: one command produces a usable forum. A cloned
	// worktree keeps the source's own credentials.
	return &InitOptions{
		TargetPath:       result.Path,
		InstanceName:     result.Instance,
		ExistingOnly:     true,
		SkipUp:           flagWorktreeNoUp,
		StartContainers:  !flagWorktreeNoUp,
		SkipInstall:      cloning,
		AdminUser:        defaultString(flagWorktreeAdminUser, defaultWorktreeAdminUser),
		AdminPassword:    defaultString(flagWorktreeAdminPassword, defaultWorktreeAdminPassword),
		AdminEmail:       defaultString(flagWorktreeAdminEmail, defaultWorktreeAdminEmail),
		SiteTitle:        defaultString(flagWorktreeTitle, result.Branch),
		EnvResolved:      map[string]string{},
		EnvSources:       map[string]string{},
		ProductOverrides: map[string]int{},
		ProductTitleMap:  map[string]string{},
	}
}

// setUpWorktree initialises the environment by delegating to init, which
// already handles Docker configuration, containers, Composer and installation.
func setUpWorktree(ctx context.Context, result *worktree.Result, opts *InitOptions) error {
	if err := initExisting(ctx, opts); err != nil {
		return fmt.Errorf("worktree created at %s, but setting up its environment failed: %w", result.Path, err)
	}

	return nil
}

func runWorktreePath(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := worktree.SourceCheckout(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	target, err := worktree.ResolveExistingPath(source, args[0])
	if err != nil {
		return err
	}

	// Printed bare, with no decoration, so it can be used directly in a shell.
	fmt.Println(target)

	return nil
}

func runWorktreeList(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := worktree.SourceCheckout(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	registry, err := worktree.NewRegistry()
	if err != nil {
		return err
	}

	entries, err := registry.ForSource(source)
	if err != nil {
		return err
	}

	return printWorktrees(entries)
}

func runWorktreeListAll(cmd *cobra.Command, args []string) error {
	registry, err := worktree.NewRegistry()
	if err != nil {
		return err
	}

	entries, err := registry.All()
	if err != nil {
		return err
	}

	return printWorktrees(entries)
}

func runWorktreeRemove(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := worktree.SourceCheckout(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	target, err := worktree.ResolveExistingPath(source, args[0])
	if err != nil {
		return err
	}

	// The path is derived from the branch's last segment, so dev/a/foo and
	// dev/b/foo resolve to the same directory. Removing without checking which
	// branch is actually there would destroy the wrong worktree, its branch,
	// and its volumes.
	if err := verifyWorktreeBranch(cmd.Context(), target, args[0]); err != nil {
		return err
	}

	// The safety check runs before anything is destroyed. Tearing down first
	// and refusing afterwards would report that the worktree was kept while
	// its database and volumes had already been deleted.
	if !flagWorktreeForce {
		if err := worktree.CheckRemovable(cmd.Context(), target); err != nil {
			return err
		}
	}

	// Containers must be torn down before the directory goes: compose reads
	// compose.yaml from the worktree to know what it owns, so removing the
	// files first would strand the containers and volumes.
	if !flagWorktreeKeepContainers {
		if err := destroyWorktreeEnvironment(cmd.Context(), target); err != nil {
			return err
		}
	}

	if err := worktree.Remove(cmd.Context(), source, target, flagWorktreeForce); err != nil {
		return err
	}

	registry, regErr := worktree.NewRegistry()
	if regErr != nil {
		// Reported rather than ignored: the worktree is gone but the registry
		// still lists it, and only this message tells the user why.
		ui.PrintWarning(fmt.Sprintf("Could not open the worktree registry: %v", regErr))
	} else if err := registry.Remove(target); err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not update the worktree registry: %v", err))
	}

	ui.PrintSuccess("Removed worktree " + target)

	return nil
}

func runWorktreePrune(cmd *cobra.Command, args []string) error {
	registry, err := worktree.NewRegistry()
	if err != nil {
		return err
	}

	entries, err := registry.All()
	if err != nil {
		return err
	}

	pruned := 0

	for _, entry := range entries {
		if _, statErr := os.Stat(entry.WorktreePath); os.IsNotExist(statErr) {
			if err := registry.Remove(entry.WorktreePath); err != nil {
				return err
			}

			pruned++
		}
	}

	if pruned == 0 {
		ui.PrintInfo("No stale worktree entries found.")

		return nil
	}

	ui.PrintSuccess(fmt.Sprintf("Pruned %d stale worktree %s.", pruned, plural(pruned, "entry", "entries")))

	return nil
}

// worktreeState reports whether a registered worktree still exists on disk.
func worktreeState(worktreePath string) string {
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return "missing"
	}

	return "ok"
}

// printWorktrees renders entries, reconciling them against the filesystem.
//
// The registry is a record, not the source of truth: worktrees get removed
// outside xf, so entries are checked rather than trusted.
func printWorktrees(entries []worktree.Entry) error {
	if flagWorktreeJSON {
		// The same reconciliation the table performs, so machine consumers
		// are not told about worktrees that no longer exist on disk.
		type worktreeListEntry struct {
			worktree.Entry

			State string `json:"state"`
		}

		listed := make([]worktreeListEntry, 0, len(entries))
		for _, entry := range entries {
			listed = append(listed, worktreeListEntry{
				Entry: entry,
				State: worktreeState(entry.WorktreePath),
			})
		}

		return printJSON(listed)
	}

	if len(entries) == 0 {
		ui.PrintInfo("No worktrees found.")

		return nil
	}

	headers := []string{"BRANCH", "PATH", "INSTANCE", "STATE"}
	rows := make([][]string, 0, len(entries))

	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Branch,
			shortenPath(entry.WorktreePath),
			entry.Instance,
			worktreeState(entry.WorktreePath),
		})
	}

	ui.Println(ui.NewTable(headers, rows))

	return nil
}

func recordWorktree(entry worktree.Entry) error {
	registry, err := worktree.NewRegistry()
	if err != nil {
		return err
	}

	return registry.Add(entry)
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode output: %w", err)
	}

	ui.Println(string(data))

	return nil
}

// resolveBranchArg returns the branch to create.
func resolveBranchArg(args []string) (string, error) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return "", fmt.Errorf(
			"a branch name is required, for example %q: %w",
			"xf worktree create dev/24x/feature", ErrInvalidInput,
		)
	}

	return args[0], nil
}

// shortenPath replaces the home directory with ~ for display.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}

	return path
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}

	return pluralForm
}

// defaultString returns value, or fallback when value is empty.
func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}

	return fallback
}

// verifyWorktreeBranch reports whether the worktree at path has branch checked
// out.
//
// Worktree paths are derived from a branch's last segment, so dev/a/foo and
// dev/b/foo share a directory. Without this check, removing one branch would
// silently destroy the other's worktree, branch, containers and volumes.
func verifyWorktreeBranch(ctx context.Context, worktreePath, branch string) error {
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no worktree at %s: %w", worktreePath, err)
		}

		return fmt.Errorf("failed to inspect %s: %w", worktreePath, err)
	}

	current, err := worktree.CurrentBranch(ctx, worktreePath)
	if err != nil {
		return fmt.Errorf("failed to determine the branch checked out at %s: %w", worktreePath, err)
	}

	if current != branch {
		return fmt.Errorf(
			"%s has %s checked out, not %s: refusing to remove it%.0w",
			worktreePath, current, branch, ErrInvalidInput,
		)
	}

	return nil
}

// destroyWorktreeEnvironment removes a worktree's containers and volumes.
//
// A worktree that was never set up has no compose configuration, which is not
// an error: there is simply nothing to tear down.
func destroyWorktreeEnvironment(ctx context.Context, worktreePath string) error {
	runner, err := dockercompose.NewRunner(worktreePath)
	if err != nil {
		// A worktree that was never set up has nothing to tear down, and a
		// directory that is already gone cannot have running containers.
		if errors.Is(err, dockercompose.ErrEnvNotInitialized) || errors.Is(err, os.ErrNotExist) {
			return nil
		}

		// Any other failure means the environment could not be inspected, not
		// that it is absent. Continuing would delete the worktree and strand
		// its containers and volumes, so stop instead.
		return fmt.Errorf("failed to inspect the worktree environment: %w", err)
	}

	spinner := ui.NewSpinner("Removing containers and volumes...")
	spinner.Start()

	if err := runner.Destroy(ctx); err != nil {
		spinner.StopWithMessage("error", "Failed to remove containers")

		return fmt.Errorf("failed to remove the worktree environment: %w", err)
	}

	spinner.StopWithMessage("success", "Containers and volumes removed")

	return nil
}

// sourceIsInstalled reports whether the source checkout holds a XenForo
// installation that can be cloned.
//
// A checkout that has never been installed has no database or attachments to
// copy, so a new worktree gets a fresh install instead.
func sourceIsInstalled(sourcePath string) (bool, error) {
	// XenForo writes this once installation completes.
	markers := []string{
		filepath.Join(sourcePath, "internal_data", "install-lock.php"),
		// Without compose configuration there is no database to dump.
		filepath.Join(sourcePath, "compose.yaml"),
	}

	for _, marker := range markers {
		if _, err := os.Stat(marker); err != nil {
			if os.IsNotExist(err) {
				return false, nil
			}

			// A permission or I/O failure is not an absent marker. Treating it
			// as one would quietly install a fresh forum where the user asked
			// for a clone of an existing one.
			return false, fmt.Errorf("failed to inspect %s: %w", marker, err)
		}
	}

	return true, nil
}
