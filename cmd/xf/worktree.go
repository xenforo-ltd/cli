package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

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

  cd "$(xf worktree path dev/24x/feature)"`,
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

var (
	flagWorktreeBase     string
	flagWorktreeNoSetup  bool
	flagWorktreeNoUp     bool
	flagWorktreeInstance string
	flagWorktreeJSON     bool
	flagWorktreeForce    bool
)

func init() {
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeBase, "base", "", "ref to branch from (defaults to current HEAD)")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeNoSetup, "no-setup", false, "create the worktree only, without setting up the environment")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeNoUp, "no-up", false, "configure the environment but do not start containers")
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeInstance, "instance", "", "Docker instance name")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "output as JSON")

	worktreeListCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "output as JSON")
	worktreeListAllCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "output as JSON")
	worktreeRemoveCmd.Flags().BoolVar(&flagWorktreeForce, "force", false, "remove even if there are uncommitted changes or unpushed commits")

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

	if !flagWorktreeNoSetup {
		if err := setUpWorktree(cmd.Context(), result); err != nil {
			return err
		}
	}

	if flagWorktreeJSON {
		return printJSON(worktreeOutput{
			Path:         result.Path,
			Branch:       result.Branch,
			SourcePath:   result.SourcePath,
			SourceBranch: result.SourceBranch,
			Instance:     result.Instance,
			CreatedAt:    result.CreatedAt,
		})
	}

	return nil
}

// setUpWorktree initialises the environment by delegating to init, which
// already handles Docker configuration, containers, Composer and installation.
func setUpWorktree(ctx context.Context, result *worktree.Result) error {
	opts := &InitOptions{
		TargetPath:       result.Path,
		InstanceName:     result.Instance,
		ExistingOnly:     true,
		SkipUp:           flagWorktreeNoUp,
		StartContainers:  !flagWorktreeNoUp,
		EnvResolved:      map[string]string{},
		EnvSources:       map[string]string{},
		ProductOverrides: map[string]int{},
		ProductTitleMap:  map[string]string{},
	}

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

	// Printed bare, with no decoration, so it can be used directly in a shell.
	fmt.Println(worktree.ResolvePath(source, args[0]))

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

	target := worktree.ResolvePath(source, args[0])

	if err := worktree.Remove(cmd.Context(), source, target, flagWorktreeForce); err != nil {
		return err
	}

	if registry, regErr := worktree.NewRegistry(); regErr == nil {
		if err := registry.Remove(target); err != nil {
			ui.PrintWarning(fmt.Sprintf("Could not update the worktree registry: %v", err))
		}
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

// printWorktrees renders entries, reconciling them against the filesystem.
//
// The registry is a record, not the source of truth: worktrees get removed
// outside xf, so entries are checked rather than trusted.
func printWorktrees(entries []worktree.Entry) error {
	if flagWorktreeJSON {
		return printJSON(entries)
	}

	if len(entries) == 0 {
		ui.PrintInfo("No worktrees found.")

		return nil
	}

	headers := []string{"BRANCH", "PATH", "INSTANCE", "STATE"}
	rows := make([][]string, 0, len(entries))

	for _, entry := range entries {
		state := "ok"
		if _, err := os.Stat(entry.WorktreePath); os.IsNotExist(err) {
			state = "missing"
		}

		rows = append(rows, []string{
			entry.Branch,
			shortenPath(entry.WorktreePath),
			entry.Instance,
			state,
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
