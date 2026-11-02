package worktree

import (
	"github.com/xenforo-ltd/cli/internal/setup"
)

const (
	// defaultWorktreeAdminUser is the default user for a fresh worktree installation.
	defaultWorktreeAdminUser = "admin"
	// defaultWorktreeAdminPassword is the default password for a fresh installation.
	defaultWorktreeAdminPassword = "password"
	// defaultWorktreeAdminEmail is the default email for a fresh installation.
	defaultWorktreeAdminEmail = "admin@example.com"
)

// CreateOptions controls the base revision, environment setup, credentials,
// instance, output format, and cloning behavior of a new worktree.
type CreateOptions struct {
	Base, AdminUser, AdminPassword, AdminEmail, Title, Instance string
	NoSetup, NoUp, JSON, Fresh, SkipBuildJS                     bool
}

// RemoveOptions controls forced removal, container retention, and error detail.
type RemoveOptions struct {
	Force          bool
	KeepContainers bool
	Verbose        bool
}

// worktreeInitOptions builds the init options for a new worktree.
//
// cloning reports whether the source environment will be copied in, which
// suppresses xf:install: the imported database is already installed, and
// reinstalling would wipe the data that was just copied. Fresh installations
// use fixed default credentials unless overridden; clones retain source logins.
func worktreeInitOptions(result *backendResult, cloning bool, opts CreateOptions) *setup.Options {
	return &setup.Options{
		TargetPath:      result.path,
		InstanceName:    result.instance,
		StartContainers: !opts.NoUp,
		SkipInstall:     cloning,
		SkipBuildJS:     opts.SkipBuildJS,
		AdminUser:       defaultString(opts.AdminUser, defaultWorktreeAdminUser),
		AdminPassword:   defaultString(opts.AdminPassword, defaultWorktreeAdminPassword),
		AdminEmail:      defaultString(opts.AdminEmail, defaultWorktreeAdminEmail),
		SiteTitle:       defaultString(opts.Title, result.branch),
		EnvResolved:     map[string]string{},
	}
}
