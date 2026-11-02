package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/commands/worktree"
)

var (
	// flagWorktreeBase selects the source revision for a new checkout.
	flagWorktreeBase string
	// flagWorktreeAdminUser sets the new installation's administrator name.
	flagWorktreeAdminUser string
	// flagWorktreeAdminPassword sets the new installation's administrator password.
	flagWorktreeAdminPassword string
	// flagWorktreeAdminEmail sets the new installation's administrator email.
	flagWorktreeAdminEmail string
	// flagWorktreeTitle sets the new installation's site title.
	flagWorktreeTitle string
	// flagWorktreeNoSetup skips environment setup after creating a checkout.
	flagWorktreeNoSetup bool
	// flagWorktreeNoUp skips starting containers during setup.
	flagWorktreeNoUp bool
	// flagWorktreeInstance selects the Docker instance name.
	flagWorktreeInstance string
	// flagWorktreeJSON selects JSON output for create or list operations.
	flagWorktreeJSON bool
	// flagWorktreeForce permits removal despite uncommitted or unpushed work.
	flagWorktreeForce bool
	// flagWorktreeKeepContainers preserves containers and volumes on removal.
	flagWorktreeKeepContainers bool
	// flagWorktreeFresh selects a fresh installation instead of cloning the source.
	flagWorktreeFresh bool
	// flagWorktreeSkipBuildJS skips JavaScript builds during setup.
	flagWorktreeSkipBuildJS bool
)

// newWorktreeCommand builds the worktree command family. The parent treats help
// as a help request and rejects other unknown subcommands rather than treating
// them as branch names. JSON creation requires --no-setup because setup writes
// progress to stdout.
func newWorktreeCommand() *cobra.Command {
	worktreeCmd := &cobra.Command{
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
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}

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

	worktreeCreateCmd := &cobra.Command{
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

	worktreeListCmd := &cobra.Command{
		Use:   "list",
		Short: "List worktrees for this project",
		Args:  cobra.NoArgs,
		RunE:  runWorktreeList,
	}

	worktreeListAllCmd := &cobra.Command{
		Use:   "list-all",
		Short: "List worktrees across all known projects",
		Args:  cobra.NoArgs,
		RunE:  runWorktreeListAll,
	}

	worktreePathCmd := &cobra.Command{
		Use:   "path <branch|name>",
		Short: "Print the path of a worktree",
		Long: `Print the path of an existing worktree, given its branch or its name.

Useful for shell and agent use:

  cd "$(xf worktree path dev/24x/feature)"
  cd "$(xf worktree path feature)"`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: worktree.CompletionRefs,
		RunE:              runWorktreePath,
	}

	worktreeRemoveCmd := &cobra.Command{
		Use:   "remove <branch|name>",
		Short: "Remove a worktree and its containers",
		Long: `Remove a worktree or workspace and its Docker containers and volumes.

For a Git repository the worktree's branch is deleted too. Refuses when the
checkout contains uncommitted changes or commits that exist on no remote, listing
what would be lost. Use --force to remove it anyway.`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: worktree.CompletionRefs,
		RunE:              runWorktreeRemove,
	}

	worktreePruneCmd := &cobra.Command{
		Use:   "prune",
		Short: "Drop registry entries for worktrees that no longer exist",
		Args:  cobra.NoArgs,
		RunE:  runWorktreePrune,
	}

	worktreeCreateCmd.Flags().StringVar(&flagWorktreeBase, "base", "", "revision to base the new checkout on, in the source VCS's syntax (defaults to current HEAD)")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeNoSetup, "no-setup", false, "create the worktree only, without setting up the environment")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeNoUp, "no-up", false, "configure the environment but do not start containers")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeFresh, "fresh", false, "install a clean forum instead of cloning the source environment")
	worktreeCreateCmd.Flags().BoolVar(&flagWorktreeSkipBuildJS, "skip-build-js", false, "skip building JavaScript for XF and add-ons")
	worktreeCreateCmd.Flags().StringVar(&flagWorktreeInstance, "instance", "", "Docker instance name")
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

	return worktreeCmd
}

// runWorktreeCreate creates a checkout with the selected setup options.
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

// runWorktreeList lists worktrees for the current project.
func runWorktreeList(cmd *cobra.Command, _ []string) error {
	return worktree.List(cmd.Context(), flagWorktreeJSON)
}

// runWorktreeListAll lists worktrees across known projects.
func runWorktreeListAll(_ *cobra.Command, _ []string) error {
	return worktree.ListAll(flagWorktreeJSON)
}

// runWorktreePath prints the path of the named worktree.
func runWorktreePath(cmd *cobra.Command, args []string) error {
	return worktree.Path(cmd.Context(), args[0])
}

// runWorktreeRemove removes the named worktree with the selected safeguards.
func runWorktreeRemove(cmd *cobra.Command, args []string) error {
	return worktree.RemoveWorktree(cmd.Context(), args[0], worktree.RemoveOptions{
		Force:          flagWorktreeForce,
		KeepContainers: flagWorktreeKeepContainers,
		Verbose:        viper.GetBool("verbose"),
	})
}

// runWorktreePrune removes registry entries for missing worktrees.
func runWorktreePrune(_ *cobra.Command, _ []string) error { return worktree.Prune() }
