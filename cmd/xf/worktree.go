package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/commands/worktree"
)

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Create and manage development worktrees",
	Long: `Create a Git worktree or Jujutsu workspace with a fully configured XenForo environment.

A worktree is a second checkout of the same repository on its own branch (Git) or
workspace (Jujutsu), with its own Docker containers and database. It lets you work
on a feature without disturbing your main checkout.

The source repository selects the backend: a repository that uses Jujutsu --
including one colocated with Git -- creates a Jujutsu workspace, while a plain
Git repository creates a Git worktree. For Jujutsu, <branch> names the workspace.

Worktrees are created alongside the source checkout, so ~/Sites/main gains
~/Sites/main.worktrees/<branch>. The path is derived from the branch name and is
always predictable.

'xf worktree create <branch>' creates the worktree and then initialises the
environment: Docker configuration, containers, Composer dependencies and the
XenForo installation.`,
	Example: `  # Create a worktree and set up its environment
  xf worktree create dev/24x/feature

  # Branch from somewhere other than the current HEAD
  xf worktree create dev/24x/feature --base main

  # Create the worktree without setting anything up
  xf worktree create dev/24x/feature --no-setup

  # Print a machine-readable result (requires --no-setup)
  xf worktree create dev/24x/feature --no-setup --json

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

		return fmt.Errorf("unknown command %q for %q: %w", args[0], cmd.CommandPath(), clierror.ErrInvalidInput)
	},
	GroupID: "env",
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

var worktreeCreateCmd = &cobra.Command{
	Use:   "create <branch>",
	Short: "Create a worktree and set up its environment",
	Long: `Create a worktree and set up its environment.

By default the new worktree's Docker environment is created and the source
environment is cloned into it. Use --no-setup to create only the worktree.
JavaScript is built for XF and add-ons with build.json, even with --no-up.
Use --skip-build-js to skip these builds.

--json requires --no-setup: setup writes progress to stdout, which would
corrupt the JSON document. With both flags, exactly one JSON document is
written to stdout.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorktreeCreate,
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
	Use:   "path <branch|name>",
	Short: "Print the path of a worktree",
	Long: `Print the path of an existing worktree, given its branch or its name.

Useful for shell and agent use:

  cd "$(xf worktree path dev/24x/feature)"
  cd "$(xf worktree path feature)"`,
	Args: cobra.ExactArgs(1),
	RunE: runWorktreePath,
}

var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <branch|name>",
	Short: "Remove a worktree and its containers",
	Long: `Remove a worktree or workspace and its Docker containers and volumes.

For a Git repository the worktree's branch is deleted too. Refuses when the
checkout contains uncommitted changes or commits that exist on no remote, listing
what would be lost. Use --force to remove it anyway.`,
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
	flagWorktreeSkipBuildJS    bool
)

func init() {
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeBase, "base", "", "revision to base the new checkout on, in the source VCS's syntax (defaults to current HEAD)")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeNoSetup, "no-setup", false, "create the worktree only, without setting up the environment")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeNoUp, "no-up", false, "configure the environment but do not start containers")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeFresh, "fresh", false, "install a clean forum instead of cloning the source environment")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeSkipBuildJS, "skip-build-js", false, "skip building JavaScript for XF and add-ons")
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeInstance, "instance", "", "Docker instance name")
	// JSON must be the only thing on stdout, but setup writes progress there.
	// Requiring --no-setup keeps the stream machine-readable without routing
	// every progress message through an injectable writer.
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "output as JSON (requires --no-setup)")
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

func runWorktreeCreate(cmd *cobra.Command, args []string) error {
	return worktree.CreateWorktree(cmd.Context(), args, worktree.CreateOptions{
		Base:          flagWorktreeBase,
		AdminUser:     flagWorktreeAdminUser,
		AdminPassword: flagWorktreeAdminPassword,
		AdminEmail:    flagWorktreeAdminEmail,
		Title:         flagWorktreeTitle,
		Instance:      flagWorktreeInstance,
		NoSetup:       flagWorktreeNoSetup,
		NoUp:          flagWorktreeNoUp,
		JSON:          flagWorktreeJSON,
		Fresh:         flagWorktreeFresh,
		SkipBuildJS:   flagWorktreeSkipBuildJS,
	})
}
func runWorktreeList(cmd *cobra.Command, _ []string) error {
	return worktree.List(cmd.Context(), flagWorktreeJSON)
}
func runWorktreeListAll(_ *cobra.Command, _ []string) error {
	return worktree.ListAll(flagWorktreeJSON)
}
func runWorktreePath(cmd *cobra.Command, args []string) error {
	return worktree.Path(cmd.Context(), args[0])
}
func runWorktreeRemove(cmd *cobra.Command, args []string) error {
	return worktree.RemoveWorktree(cmd.Context(), args[0], worktree.RemoveOptions{
		Force:          flagWorktreeForce,
		KeepContainers: flagWorktreeKeepContainers,
		Verbose:        viper.GetBool("verbose"),
	})
}
func runWorktreePrune(_ *cobra.Command, _ []string) error { return worktree.Prune() }
