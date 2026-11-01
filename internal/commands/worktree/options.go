package worktree

import (
	"context"
	"fmt"

	"github.com/xenforo-ltd/cli/internal/setup"
)

// Defaults for the throwaway installation a worktree gets.
const (
	defaultWorktreeAdminUser     = "admin"
	defaultWorktreeAdminPassword = "password"
	defaultWorktreeAdminEmail    = "admin@example.com"
)

type CreateOptions struct {
	Base, AdminUser, AdminPassword, AdminEmail, Title, Instance string
	NoSetup, NoUp, JSON, Fresh, SkipBuildJS                     bool
}

type RemoveOptions struct {
	Force          bool
	KeepContainers bool
	Verbose        bool
}

// worktreeInitOptions builds the init options for a new worktree.
//
// cloning reports whether the source environment will be copied in, which
// suppresses xf:install: the imported database is already installed, and
// reinstalling would wipe the data that was just copied.
func worktreeInitOptions(result *backendResult, cloning bool, opts CreateOptions) *setup.Options {
	// A worktree is a disposable development environment, so a fresh install
	// uses fixed credentials rather than prompting. Knowing the login without
	// being asked is the point: one command produces a usable forum. A cloned
	// worktree keeps the source's own credentials.
	return &setup.Options{
		TargetPath:      result.Path,
		InstanceName:    result.Instance,
		StartContainers: !opts.NoUp,
		SkipInstall:     cloning,
		SkipBuildJS:     opts.SkipBuildJS,
		AdminUser:       defaultString(opts.AdminUser, defaultWorktreeAdminUser),
		AdminPassword:   defaultString(opts.AdminPassword, defaultWorktreeAdminPassword),
		AdminEmail:      defaultString(opts.AdminEmail, defaultWorktreeAdminEmail),
		SiteTitle:       defaultString(opts.Title, result.Branch),
		EnvResolved:     map[string]string{},
	}
}

// setUpWorktree initialises the environment by delegating to setup, which
// already handles Docker configuration, containers, Composer, JavaScript and installation.
func setUpWorktree(ctx context.Context, result *backendResult, opts *setup.Options) error {
	if err := setup.Existing(ctx, opts); err != nil {
		return fmt.Errorf("worktree created at %s, but setting up its environment failed: %w", result.Path, err)
	}

	return nil
}
