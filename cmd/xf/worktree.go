package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
XenForo installation.

'xf worktree adopt <path>' registers an existing linked worktree or workspace
that xf did not create, so list, list-all and prune can see it and path and
remove can address it wherever it lives. Adoption records metadata only: it
never changes the checkout or its environment.`,
	Example: `  # Create a worktree and set up its environment
  xf worktree create dev/24x/feature

  # Branch from somewhere other than the current HEAD
  xf worktree create dev/24x/feature --base main

  # Create the worktree without setting anything up
  xf worktree create dev/24x/feature --no-setup

  # Print a machine-readable result (requires --no-setup)
  xf worktree create dev/24x/feature --no-setup --json

  # Register an existing worktree or workspace at any location
  xf worktree adopt ../shared/feature

  # Register it and record a Docker instance name
  xf worktree adopt ../shared/feature --instance my-feature

  # Print the registration as JSON
  xf worktree adopt ../shared/feature --json

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

var worktreeAdoptCmd = &cobra.Command{
	Use:   "adopt <path>",
	Short: "Register an existing worktree or workspace",
	Long: `Register an existing Git worktree or Jujutsu workspace that xf did not create.

Adoption is metadata-only. It reads the checkout's root, source and branch or
workspace name, then records them so list, list-all, path and remove can find a
checkout that lives outside the usual "<source>.worktrees/<branch>" location.
It does not change any file in the checkout, touch version-control metadata,
create or move branches, bookmarks or workspaces, configure Docker, or set up
an environment.

<path> may be the checkout's root or any directory inside it, absolute or
relative to your current directory. The checkout must be a XenForo checkout
with a regular src/XF.php. The source or main checkout cannot be adopted, and a
Git checkout in a detached HEAD state is refused. A Jujutsu workspace named
HEAD is not detached and is accepted.

The Docker instance name comes from --instance when given, otherwise from a
non-empty XF_INSTANCE in the checkout's .env, otherwise it is derived from the
branch or workspace name, without a collision suffix. It is recorded only:
adoption does not write .env or move containers, so a later remove still reads
the checkout's own configuration.

Re-adopting a path refreshes its record, including the registration time and
instance, and clears the "cloned" flag; it never infers how the checkout was
originally made.`,
	Args: cobra.ExactArgs(1),
	RunE: runWorktreeAdopt,
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
  cd "$(xf worktree path feature)"

For an adopted checkout at a non-conventional location, the recorded path is
printed instead when the conventional one does not exist. A registered path
that is missing still prints, so it remains visible as Missing in list until
prune drops the record.`,
	Args: cobra.ExactArgs(1),
	RunE: runWorktreePath,
}

var worktreeRemoveCmd = &cobra.Command{
	Use:   "remove <branch|name>",
	Short: "Remove a worktree and its containers",
	Long: `Remove a worktree or workspace and its Docker containers and volumes.

For a Git repository the worktree's branch is deleted too. Refuses when the
checkout contains uncommitted changes or commits that exist on no remote, listing
what would be lost. Use --force to remove it anyway.

This works for an adopted checkout at a non-conventional location. Removal is
destructive for adopted checkouts just as for created ones, and it verifies that
the target really is a registered, non-source checkout of this repository before
deleting anything -- even with --force. An adopted record's instance name does
not retarget containers: removal reads the checkout's own Docker configuration.`,
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
	flagWorktreeSkipBuildJS    bool
)

// init registers the worktree commands and their flags.
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
	worktreeAdoptCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "output as JSON")
	worktreeAdoptCmd.Flags().StringVar(&flagWorktreeInstance, "instance", "", "Docker instance name to record for the checkout")
	worktreeRemoveCmd.Flags().BoolVar(&flagWorktreeForce, "force", false, "remove even if there are uncommitted changes or unpushed commits")
	worktreeRemoveCmd.Flags().BoolVar(&flagWorktreeKeepContainers, "keep-containers", false, "leave the Docker containers and volumes in place")

	worktreeCmd.AddCommand(worktreeCreateCmd)
	worktreeCmd.AddCommand(worktreeAdoptCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeListAllCmd)
	worktreeCmd.AddCommand(worktreePathCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)
	worktreeCmd.AddCommand(worktreePruneCmd)

	rootCmd.AddCommand(worktreeCmd)
}

// worktreeOutput is the machine-readable form of a created or adopted
// worktree.
type worktreeOutput struct {
	Path         string    `json:"path"`
	Branch       string    `json:"branch"`
	SourcePath   string    `json:"source_path"`
	SourceBranch string    `json:"source_branch"`
	Instance     string    `json:"instance"`
	Cloned       bool      `json:"cloned"`
	CreatedAt    time.Time `json:"created_at"`
}

// runWorktreeCreate creates a worktree and, unless --no-setup is set, sets up
// its environment.
func runWorktreeCreate(cmd *cobra.Command, args []string) error {
	// JSON must be the only thing on stdout, but setup writes progress there.
	// Requiring --no-setup keeps the stream machine-readable. This runs before
	// any path resolution or Git state changes so a rejected invocation leaves
	// nothing behind.
	if flagWorktreeJSON && !flagWorktreeNoSetup {
		return newUsageError(fmt.Errorf(
			"--json requires --no-setup: setup progress is written to stdout and would corrupt the JSON document",
		))
	}

	branch, err := resolveBranchArg(args)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Use the same source policy as list, path and remove. For an adopted
	// Jujutsu workspace at an arbitrary location that is its repository host,
	// so a child created here is recorded under the source the management
	// commands later consult. Resolving the conventional source instead would
	// place the child beside the adopted workspace and leave it unfindable.
	source, err := worktreeManagementSource(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	registry, err := worktree.NewRegistry()
	if err != nil {
		return err
	}

	reserved, err := reserveWorktreeName(cmd.Context(), registry, source, branch)
	if err != nil {
		return err
	}

	result, err := worktree.Create(cmd.Context(), worktree.Options{
		SourcePath: source,
		Branch:     branch,
		Name:       filepath.Base(reserved.WorktreePath),
		Base:       flagWorktreeBase,
		Instance:   reserved.Instance,
	})
	if err != nil {
		// Release the name so a retry can use it, unless creation left the
		// directory behind. ChooseName skips that directory for this source,
		// but only the reservation keeps another checkout from reusing the
		// instance name.
		if _, statErr := os.Lstat(reserved.WorktreePath); errors.Is(statErr, fs.ErrNotExist) {
			if removeErr := registry.Remove(reserved.WorktreePath); removeErr != nil {
				warnRegistryUpdateFailed()
			}
		}

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
		warnRegistryUpdateFailed()
	}

	if !flagWorktreeJSON {
		ui.SuccessBox("Created worktree", []ui.KVPair{
			ui.KV("Path", ui.Path.Render(ui.ShortHome(result.Path))),
			ui.KV("Branch", result.Branch),
			ui.KV("Base branch", result.SourceBranch),
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

	// --no-up on an installed source leaves the worktree configured but with
	// neither a cloned database nor an installed one, and nothing about a
	// successful "create" says so. A later "xf up" would then present an empty
	// board with no explanation, so name the gap and the way to close it.
	if flagWorktreeNoUp && !flagWorktreeFresh && !flagWorktreeNoSetup && !flagWorktreeJSON {
		installed, err := sourceIsInstalled(result.SourcePath)
		if err != nil {
			return err
		}

		if installed {
			ui.PrintWarning("--no-up skips cloning the source environment, so this worktree has no database yet. " +
				"Run xf worktree create without --no-up to clone it, or xf up and then install manually.")
		}
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
				warnRegistryUpdateFailed()
			}
		}

		// Only a fresh install has credentials worth reporting. A cloned
		// worktree keeps the source's own logins, so printing the defaults
		// would be wrong.
		if !cloning && !flagWorktreeJSON && !flagWorktreeNoUp {
			ui.Println()
			ui.InfoBox("Fresh install credentials", []ui.KVPair{
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
		SkipBuildJS:      flagWorktreeSkipBuildJS,
		AdminUser:        defaultString(flagWorktreeAdminUser, defaultWorktreeAdminUser),
		AdminPassword:    defaultString(flagWorktreeAdminPassword, defaultWorktreeAdminPassword),
		AdminEmail:       defaultString(flagWorktreeAdminEmail, defaultWorktreeAdminEmail),
		SiteTitle:        defaultString(flagWorktreeTitle, result.Branch),
		EnvResolved:      map[string]string{},
		ProductOverrides: map[string]int{},
		ProductTitleMap:  map[string]string{},
	}
}

// setUpWorktree initialises the environment by delegating to init, which
// already handles Docker configuration, containers, Composer, JavaScript and installation.
func setUpWorktree(ctx context.Context, result *worktree.Result, opts *InitOptions) error {
	if err := initExisting(ctx, opts); err != nil {
		return fmt.Errorf("worktree created at %s, but setting up its environment failed: %w", result.Path, err)
	}

	return nil
}

// runWorktreeAdopt registers an existing checkout in the registry, recording
// the instance name resolved from --instance, the local .env or the branch.
func runWorktreeAdopt(cmd *cobra.Command, args []string) error {
	// A blank path is a usage mistake. Adopt rejects it too, but catching it at
	// the boundary means usage is printed for a mistyped invocation.
	if strings.TrimSpace(args[0]) == "" {
		return newUsageError(fmt.Errorf("a path is required, for example %q", "xf worktree adopt ../feature"))
	}

	result, err := worktree.Adopt(cmd.Context(), worktree.AdoptOptions{
		Path:     args[0],
		Instance: flagWorktreeInstance,
	})
	if err != nil {
		return err
	}

	instance := result.Instance
	if flagWorktreeInstance == "" {
		// A local XF_INSTANCE names the environment that already exists, so it
		// beats the derived name. A genuine read failure stops registration
		// rather than recording a guessed instance.
		local, err := dockercompose.ReadInstanceName(result.Path)
		if err != nil {
			return err
		}

		if local != "" {
			instance = local
		}
	}

	// No other useful operation has happened, so a registry failure is a hard
	// error rather than a warning printed after a successful action.
	if err := cmd.Context().Err(); err != nil {
		return err
	}

	entry := worktree.Entry{
		SourcePath:   result.SourcePath,
		SourceBranch: result.SourceBranch,
		WorktreePath: result.Path,
		Branch:       result.Branch,
		Instance:     instance,
		CreatedAt:    result.CreatedAt,
	}

	if err := recordWorktree(entry); err != nil {
		return fmt.Errorf("failed to register the worktree: %w", err)
	}

	if flagWorktreeJSON {
		return printJSON(worktreeOutput{
			Path:         result.Path,
			Branch:       result.Branch,
			SourcePath:   result.SourcePath,
			SourceBranch: result.SourceBranch,
			Instance:     instance,
			Cloned:       false,
			CreatedAt:    result.CreatedAt,
		})
	}

	ui.SuccessBox("Registered worktree", []ui.KVPair{
		ui.KV("Path", ui.Path.Render(ui.ShortHome(result.Path))),
		ui.KV("Branch", result.Branch),
		ui.KV("Source", ui.Path.Render(ui.ShortHome(result.SourcePath))),
		ui.KV("Instance", instance),
	})

	return nil
}

// runWorktreePath prints the path of an existing worktree.
func runWorktreePath(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := worktreeManagementSource(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	target, err := worktreeTarget(cmd.Context(), source, args[0])
	if err != nil {
		return err
	}

	// Printed bare, with no decoration, so it can be used directly in a shell.
	fmt.Println(target)

	return nil
}

// runWorktreeList prints the worktrees recorded for the current source.
func runWorktreeList(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := worktreeManagementSource(cmd.Context(), cwd)
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
		return registryReadError(err)
	}

	return printWorktrees(entries)
}

// runWorktreeRemove validates a worktree and removes it with its environment.
func runWorktreeRemove(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	source, err := worktreeManagementSource(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	target, err := worktreeTarget(cmd.Context(), source, args[0])
	if err != nil {
		return err
	}

	// A registry-selected target may live anywhere, so confirm it really is a
	// registered, non-source checkout of this repository before anything is
	// destroyed. This holds even under --force: force widens what the version
	// control system discards, never whose checkout is deleted.
	//
	// The argument may name the worktree's directory rather than its branch,
	// and Locate has already matched that name to the conventional directory.
	// Compare it with the owner only when it is not that name, so removing a
	// numbered directory such as "feature-2" is not refused for the branch it
	// holds. A registry-selected adopted checkout keeps the strict comparison.
	expected := args[0]
	if filepath.Base(target) == args[0] &&
		comparablePath(filepath.Dir(target)) == comparablePath(worktree.WorktreesDir(source)) {
		expected = ""
	}

	if err := worktree.ValidateRemovalTarget(cmd.Context(), source, target, expected); err != nil {
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
		warnRegistryUpdateFailed()
		printCause(regErr)
	} else if err := registry.Remove(target); err != nil {
		warnRegistryUpdateFailed()
		printCause(err)
	}

	ui.SuccessBox("Removed worktree "+ui.Path.Render(ui.ShortHome(target)), nil)

	return nil
}

func runWorktreePrune(cmd *cobra.Command, args []string) error {
	registry, err := worktree.NewRegistry()
	if err != nil {
		return err
	}

	pruned, err := registry.Prune(time.Now())
	if err != nil {
		return registryReadError(err)
	}

	if pruned == 0 {
		ui.PrintEmpty("No stale worktree entries found")

		return nil
	}

	ui.PrintSuccess(fmt.Sprintf("Pruned %s", ui.Plural(pruned, "stale worktree entry", "stale worktree entries")))

	return nil
}

// worktreeState reports whether a registered worktree still exists on disk.
//
// The value is unstyled so it can go straight into JSON; renderWorktreeState
// colours it for the table.
func worktreeState(worktreePath string) string {
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}

		// A permission or I/O failure says nothing about whether the worktree
		// is there. Reporting "ok" would claim it is usable on the strength of
		// a check that never completed.
		return "unknown"
	}

	return "ok"
}

// renderWorktreeState colours a worktree's state for display.
//
// Anything that is not a confirmed "ok" (including "unknown" from a failed
// stat) renders as a warning: the table must never claim a worktree is usable
// on the strength of a check that never completed.
func renderWorktreeState(state string) string {
	switch state {
	case "ok":
		return ui.Success.Render("Ready")
	case "missing":
		return ui.Warning.Render("Missing")
	default:
		return ui.Warning.Render("Unknown")
	}
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
		ui.PrintEmpty("No worktrees found")

		return nil
	}

	headers := []string{"BRANCH", "PATH", "INSTANCE", "STATE"}
	rows := make([][]string, 0, len(entries))

	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Branch,
			shortenPath(entry.WorktreePath),
			entry.Instance,
			renderWorktreeState(worktreeState(entry.WorktreePath)),
		})
	}

	ui.PrintInfo(ui.Plural(len(rows), "worktree", "worktrees"))
	ui.Println()
	ui.Println(ui.NewTable(headers, rows))

	return nil
}

// registryReadError explains how to recover from an unreadable registry.
//
// The registry is only a record, so deleting it cannot damage a checkout: the
// worktrees themselves are still on disk and still known to the version control
// system. What is lost is the cross-project listing and, for adopted
// checkouts, the stored path and source that path and remove consult outside
// the conventional location. Existing linked checkouts can simply be adopted
// again once a fresh registry exists. Saying so turns a dead end into one
// command.
func registryReadError(err error) error {
	if !errors.Is(err, worktree.ErrRegistryCorrupt) {
		return err
	}

	path, pathErr := worktree.RegistryPath()
	if pathErr != nil {
		return err
	}

	return withHint(err, "Checkout files and version-control registrations are unaffected: delete "+
		ui.Path.Render(ui.ShortHome(path))+" to start a new record, then adopt any linked checkouts again")
}

// warnRegistryUpdateFailed reports a registry write failure without printing
// the underlying error: the operation it was recording already succeeded, so
// this is advisory, not a failure report.
func warnRegistryUpdateFailed() {
	ui.PrintWarning("Could not update the worktree registry")
}

// recordWorktree adds an entry to the registry.
func recordWorktree(entry worktree.Entry) error {
	registry, err := worktree.NewRegistry()
	if err != nil {
		return err
	}

	return registry.Add(entry)
}

// worktreeManagementSource resolves the source checkout that owns the worktree
// being managed or created.
//
// It starts from conventional detection, then consults the registry when the
// current directory is inside the returned source. That covers an adopted
// Jujutsu workspace at an arbitrary location, whose conventional source would
// otherwise be itself. A conventional child checkout sits beside its source
// rather than inside it, so its sibling source is never redirected merely
// because that source was also adopted.
//
// Creation resolves its source through here too, so a child created from an
// adopted workspace is recorded under the same source that list, path and
// remove later consult.
//
// A registry that cannot be opened or read leaves the conventional source in
// place; list reports that failure itself. Conflicting or stale source records
// are surfaced rather than silently ignored. A recorded source is accepted
// only when read-only discovery in the current checkout confirms it.
func worktreeManagementSource(ctx context.Context, cwd string) (string, error) {
	source, err := worktree.SourceCheckout(ctx, cwd)
	if err != nil {
		return "", err
	}

	if !pathWithin(source, cwd) {
		return source, nil
	}

	registry, err := worktree.NewRegistry()
	if err != nil {
		return source, nil
	}

	entry, ok, err := registry.ForWorktree(source)
	if err != nil {
		if errors.Is(err, worktree.ErrRegistryAmbiguous) {
			return "", err
		}

		return source, nil
	}

	if ok {
		if err := worktree.ValidateAdoptedSource(ctx, source, entry.SourcePath); err != nil {
			return "", fmt.Errorf("invalid recorded worktree source: %w", err)
		}

		return entry.SourcePath, nil
	}

	return source, nil
}

// worktreeTarget resolves the checkout that path or remove should act on.
//
// The conventional location wins: Locate returns the checkout registered under
// "<source>.worktrees/<branch>". When there is none, a registry record for the
// source supplies an adopted checkout's stored location, which is what makes a
// checkout outside the conventional location addressable. A conventional
// checkout is never overridden by a record, so removal's own branch check can
// still refuse a conventional-name collision rather than silently choosing
// some other checkout.
//
// Locate's "not found" is the only failure that falls through to the registry:
// a permission or I/O failure is returned rather than mistaken for absence.
func worktreeTarget(ctx context.Context, source, branch string) (string, error) {
	derived, err := worktree.Locate(ctx, source, branch)
	if err == nil {
		return derived, nil
	}

	if !errors.Is(err, worktree.ErrWorktreeNotFound) {
		return "", err
	}

	notFound := err

	registry, err := worktree.NewRegistry()
	if err != nil {
		return "", notFound
	}

	entries, err := registry.ForSource(source)
	if err != nil {
		return "", notFound
	}

	seen := make(map[string]bool)

	var matches []string

	for _, entry := range entries {
		if entry.Branch != branch {
			continue
		}

		id := comparablePath(entry.WorktreePath)
		if seen[id] {
			continue
		}

		seen[id] = true

		matches = append(matches, entry.WorktreePath)
	}

	switch len(matches) {
	case 0:
		return "", notFound
	case 1:
		return matches[0], nil
	}

	// Git permits two worktrees on the same branch, so more than one record is
	// possible. Choosing one arbitrarily could act on the wrong checkout.
	sort.Strings(matches)

	return "", fmt.Errorf(
		"several registered worktrees use the branch or workspace %q:\n  %s\nRun 'xf worktree prune' to drop stale entries, or correct the registry if an entry is wrong",
		branch, strings.Join(matches, "\n  "),
	)
}

// pathWithin reports whether target is base or lies inside it.
//
// Components are compared rather than a string prefix, so "/a/bc" is not
// treated as inside "/a/b".
func pathWithin(base, target string) bool {
	rel, err := filepath.Rel(comparablePath(base), comparablePath(target))
	if err != nil {
		return false
	}

	if rel == "." {
		return true
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// comparablePath resolves a path for identity comparison, falling back to the
// cleaned path when it cannot be resolved. It mirrors the worktree package's
// own identity rule so registry lookups agree with it.
func comparablePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}

	return filepath.Clean(path)
}

// printJSON writes v to stdout as indented JSON.
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

// defaultString returns value, or fallback when value is empty.
func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}

	return fallback
}

// maxReserveAttempts bounds how often reserveWorktreeName retries after
// another process claims the name it chose.
const maxReserveAttempts = 10

// reserveWorktreeName chooses a name for a new worktree of branch and records
// it in the registry, returning the reserved entry.
//
// Choosing and recording are separate steps, so another xf process can claim
// the same name in between. Reserve detects that under the registry lock, and
// the name is chosen again, now seeing the other process's entry.
//
// The worktree's name doubles as its instance name unless --instance gave one.
// An explicit instance is the user's choice, so a clash on it is reported
// rather than numbered around. The directory is still numbered.
func reserveWorktreeName(ctx context.Context, registry *worktree.Registry, source, branch string) (worktree.Entry, error) {
	sourceInstance, err := configuredInstance(source)
	if err != nil {
		return worktree.Entry{}, err
	}

	for range maxReserveAttempts {
		if flagWorktreeInstance != "" {
			taken, err := instanceTaken(ctx, registry, sourceInstance, flagWorktreeInstance)
			if err != nil {
				return worktree.Entry{}, err
			}

			if taken {
				return worktree.Entry{}, fmt.Errorf(
					"the instance name %q is already in use by another environment; choose another name with --instance",
					flagWorktreeInstance,
				)
			}
		}

		name, err := worktree.ChooseName(source, branch, func(name string) (bool, error) {
			reserved, err := directoryReserved(registry, source, name)
			if err != nil || reserved {
				return reserved, err
			}

			// With --instance the name is only a directory, so it cannot
			// clash with another environment's instance.
			if flagWorktreeInstance != "" {
				return false, nil
			}

			return instanceTaken(ctx, registry, sourceInstance, name)
		})
		if err != nil {
			return worktree.Entry{}, err
		}

		entry := worktree.Entry{
			SourcePath:   source,
			WorktreePath: filepath.Join(worktree.WorktreesDir(source), name),
			Branch:       branch,
			Instance:     defaultString(flagWorktreeInstance, name),
			CreatedAt:    time.Now().UTC(),
		}

		err = registry.Reserve(entry)
		if err == nil {
			return entry, nil
		}

		if !errors.Is(err, worktree.ErrNameReserved) {
			return worktree.Entry{}, registryReadError(err)
		}
	}

	return worktree.Entry{}, fmt.Errorf("could not reserve a worktree name for %q: other xf processes kept claiming it", branch)
}

// directoryReserved reports whether a registry entry, possibly a reservation
// for a worktree still being created, holds the directory name in source's
// worktrees directory. A reservation made with --instance holds a directory
// whose name differs from its instance, and the directory does not exist until
// the creation finishes, so neither the instance check nor the filesystem
// would see it.
func directoryReserved(registry *worktree.Registry, source, name string) (bool, error) {
	entries, err := registry.All()
	if err != nil {
		return false, registryReadError(err)
	}

	path := filepath.Clean(filepath.Join(worktree.WorktreesDir(source), name))

	for _, entry := range entries {
		if filepath.Clean(entry.WorktreePath) == path {
			return true, nil
		}
	}

	return false, nil
}

// instanceTaken reports whether an instance name is already used by another
// environment.
//
// Docker Compose projects are global, so a project left by another checkout,
// or by a worktree removed with --keep-containers, would otherwise be shared.
// The registry covers worktrees with no project yet: those created with
// --no-setup, and those still being created by another process. The source's
// configured instance is reserved even if it has never been started.
//
// Every registry entry counts, including ones whose worktree has gone, until
// "xf worktree prune" drops them: an entry can be a reservation for a worktree
// that does not exist yet, and prune leaves those alone. A registry that
// cannot be read is an error rather than an empty one, because it is what
// keeps names unique.
//
// With --no-setup Docker need not be running, so failing to reach it is not
// an error there.
func instanceTaken(ctx context.Context, registry *worktree.Registry, sourceInstance, name string) (bool, error) {
	if name == sourceInstance {
		return true, nil
	}

	entries, err := registry.All()
	if err != nil {
		return false, registryReadError(err)
	}

	for _, entry := range entries {
		if entry.Instance == name {
			return true, nil
		}
	}

	exists, err := dockercompose.ProjectExists(ctx, name)
	if err != nil {
		if flagWorktreeNoSetup {
			return false, nil
		}

		return false, err
	}

	return exists, nil
}

// configuredInstance returns the instance name the checkout at dir is
// configured to use, or an empty string when it has no environment.
func configuredInstance(dir string) (string, error) {
	runner, err := dockercompose.NewRunner(dir)
	if err != nil {
		if errors.Is(err, dockercompose.ErrEnvNotInitialized) {
			return "", nil
		}

		return "", fmt.Errorf("failed to read the source environment: %w", err)
	}

	return runner.Instance(), nil
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

	spinner := ui.NewSpinner("Removing containers and volumes")
	spinner.Start()

	if err := runner.Destroy(ctx); err != nil {
		spinner.Stop()

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
