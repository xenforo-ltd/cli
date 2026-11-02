// Package cli defines the xf command tree and its process entry point.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/xenforo-ltd/cli/internal/clierror"
	rootcommand "github.com/xenforo-ltd/cli/internal/commands/root"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/version"
)

// rootConfig carries the process inputs the command tree reads and writes.
type rootConfig struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// Run builds the command tree, dispatches args, and returns the exit status.
//
// It never calls os.Exit, so the caller owns process termination and tests can
// exercise routing and error reporting in-process.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg := rootConfig{stdin: stdin, stdout: stdout, stderr: stderr}
	root := newRootCommand(cfg)
	root.SetArgs(args)
	configureErrorHandling(root)

	if len(args) > 0 && takesDirectXenForoRoute(args[0]) && !isKnownCommand(root, args[0]) {
		if err := rootcommand.Run(ctx, args); err != nil {
			return reportError(err, nil, cfg)
		}

		return 0
	}

	executed, err := root.ExecuteContextC(ctx)
	if err != nil {
		return reportError(err, executed, cfg)
	}

	return 0
}

// reportError prints err and returns the matching exit status.
//
// Usage is printed only for invocation mistakes, where it is the helpful
// response; runtime failures such as Docker being unavailable are not.
func reportError(err error, executed *cobra.Command, cfg rootConfig) int {
	if code, quiet := quietExitCode(err); quiet {
		return code
	}

	handleError(err, cfg.stderr)

	if clierror.IsUsage(err) && executed != nil {
		_, _ = fmt.Fprintln(cfg.stderr)
		_, _ = fmt.Fprint(cfg.stderr, executed.UsageString())
	}

	return 1
}

// quietExitCode reports the status for errors that should not be reported: an
// interrupt, a cancellation, or a child's own exit status.
//
// Ctrl-C is a deliberate user action, so it exits with the conventional signal
// status rather than reporting an error.
func quietExitCode(err error) (int, bool) {
	if isInterrupted(err) {
		return interruptExitCode(), true
	}

	if errors.Is(err, clierror.ErrCancelled) {
		return 0, true
	}

	if code, ok := clierror.ExitCode(err); ok {
		return code, true
	}

	return 0, false
}

// newRootCommand builds the command tree and registers every command family.
// TraverseChildren parses root flags before passthrough commands; otherwise
// their disabled flag parsing would forward those flags to the wrapped tool.
// Arguments after a passthrough command belong to that tool. Unknown arguments
// are rejected when a leading flag prevents direct XenForo routing, rather
// than silently succeeding.
// Help styling respects the output color profile, and completion is registered
// after command groups so it remains available for dispatch.
func newRootCommand(cfg rootConfig) *cobra.Command {
	var configFile string

	root := &cobra.Command{
		Use:              "xf",
		Version:          version.Version,
		TraverseChildren: true,
		Short:            "Provision and manage XenForo development environments",
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			err := config.Init(configFile)
			if _, ok := errors.AsType[viper.ConfigFileNotFoundError](err); ok {
				return nil
			}

			return err
		},
		Long: `Provision and manage Docker-based XenForo development environments:
authentication, package downloads, caching, containers and worktrees.

Inside a XenForo directory, unknown commands are forwarded to XenForo itself,
so ` + "`xf list`" + ` runs ` + "`cmd.php list`" + ` in the environment.`,
		Example: `  # Authenticate, then create a project
  xf auth login
  xf init ./my-project

  # Run a XenForo command in the environment
  xf xf-dev:import`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			return clierror.WithHint(
				clierror.Usage(clierror.MarkAs(clierror.ErrInvalidInput, "unknown command %q for %q", args[0], cmd.CommandPath())),
				"Global flags cannot precede a XenForo command; run "+ui.Command.Render(cmd.CommandPath()+" "+args[0])+" instead",
			)
		},
	}

	root.SetIn(cfg.stdin)
	root.SetOut(cfg.stdout)
	root.SetErr(cfg.stderr)

	root.AddGroup(
		&cobra.Group{ID: "start", Title: "Getting started:"},
		&cobra.Group{ID: "env", Title: "Environment:"},
		&cobra.Group{ID: "run", Title: "Run tools:"},
		&cobra.Group{ID: "maint", Title: "Maintenance:"},
	)
	root.SetHelpCommandGroupID("maint")
	root.SetCompletionCommandGroupID("maint")

	cobra.AddTemplateFunc("styleHeading", func(s string) string {
		return lipgloss.Sprint(ui.Bold.Render(s))
	})

	cobra.AddTemplateFunc("styleCommand", func(s string) string {
		return lipgloss.Sprint(ui.Command.Render(s))
	})

	root.SetUsageTemplate(usageTemplate)

	root.PersistentFlags().StringVarP(&configFile, "config", "c", "", "path to config file")
	root.PersistentFlags().BoolP("no-interaction", "n", false, "disable interactive prompts")
	root.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")

	cobra.CheckErr(viper.BindPFlag("verbose", root.PersistentFlags().Lookup("verbose")))
	cobra.CheckErr(viper.BindPFlag("no_interaction", root.PersistentFlags().Lookup("no-interaction")))

	root.AddCommand(
		newAuthCommand(),
		newBuildCommand(),
		newCacheCommand(),
		newComposeCommand(),
		newComposerCommand(),
		newDBCommand(),
		newDebugCommand(),
		newDoctorCommand(),
		newDownCommand(),
		newDownloadCommand(),
		newExecCommand(),
		newInitCommand(),
		newLicensesCommand(),
		newLogsCommand(),
		newPHPCommand(),
		newPHPDebugCommand(),
		newPSCommand(),
		newRebootCommand(),
		newSelfUpdateCommand(),
		newUpCommand(),
		newUpdateCommand(),
		newUpgradeCommand(),
		newVersionCommand(),
		newWorktreeCommand(),
	)

	root.InitDefaultCompletionCmd()

	return root
}

// configureErrorHandling makes cobra print usage only for genuine misuse.
//
// By default cobra prints the full usage block for any error returned from RunE,
// including runtime failures such as Docker being unavailable. That wrongly
// implies the command was typed incorrectly. Usage is still printed for argument
// and flag errors, where it is the helpful response. Run prints errors itself,
// so Cobra's error output is also silenced.
func configureErrorHandling(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	if cmd.Args != nil {
		args := cmd.Args
		cmd.Args = func(c *cobra.Command, a []string) error {
			if err := args(c, a); err != nil {
				return clierror.Usage(err)
			}

			return nil
		}
	}

	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clierror.Usage(err)
	})

	for _, sub := range cmd.Commands() {
		configureErrorHandling(sub)
	}
}

// usageTemplate is cobra's default usage template with the section headings
// and command names styled via the styleHeading/styleCommand template funcs
// registered in newRootCommand. It is not otherwise restructured.
const usageTemplate = `{{styleHeading "Usage:"}}{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

{{styleHeading "Aliases:"}}
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

{{styleHeading "Examples:"}}
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

{{styleHeading "Available Commands:"}}{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{styleCommand (rpad .Name .NamePadding) }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{styleHeading .Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{styleCommand (rpad .Name .NamePadding) }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

{{styleHeading "Additional Commands:"}}{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{styleCommand (rpad .Name .NamePadding) }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

{{styleHeading "Flags:"}}
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

{{styleHeading "Global Flags:"}}
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

{{styleHeading "Additional help topics:"}}{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{styleCommand (rpad .CommandPath .CommandPathPadding)}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// exitInterrupted is the conventional exit status for a process terminated by
// SIGINT (128 + 2). It is the default when no signal was recorded, because
// Ctrl-C is the interruption users actually produce.
const exitInterrupted = 130

// interruptSignal records which signal ended the process, so the exit status
// can follow the 128+signum convention rather than always reporting SIGINT.
var interruptSignal atomic.Value

// RecordInterruptSignal stores the signal that terminated the process.
func RecordInterruptSignal(sig os.Signal) {
	interruptSignal.Store(sig)
}

// interruptExitCode returns the conventional exit status for the signal that
// ended the process: 143 for SIGTERM, 130 for SIGINT or an unrecorded signal.
func interruptExitCode() int {
	if sig, ok := interruptSignal.Load().(syscall.Signal); ok {
		return 128 + int(sig)
	}

	return exitInterrupted
}

// isInterrupted reports whether an error is the result of the user cancelling
// the command, typically with Ctrl-C.
//
// A timeout is deliberately excluded: unlike an interrupt, it is a failure the
// user did not ask for and should still be reported.
func isInterrupted(err error) bool {
	return errors.Is(err, context.Canceled)
}

// takesDirectXenForoRoute reports whether a first argument is eligible to be
// forwarded straight to XenForo, bypassing cobra.
//
// Known limitation: a leading flag is not eligible, so a global flag cannot be
// combined with a direct XenForo command. `xf -v xf-dev:import` falls through to
// cobra, where the root's RunE rejects it as an unknown command.
func takesDirectXenForoRoute(firstArg string) bool {
	if firstArg == cobra.ShellCompRequestCmd || firstArg == cobra.ShellCompNoDescRequestCmd {
		return false
	}

	return !strings.HasPrefix(firstArg, "-") &&
		firstArg != "help" && firstArg != "--help" && firstArg != "-h"
}

// handleError renders the error and any hint to w, respecting its color
// profile. Unless verbose output is enabled, it omits plumbing error clauses.
func handleError(err error, w io.Writer) {
	msg := err.Error()
	if !viper.GetBool("verbose") {
		msg = firstErrorClause(msg)
	}

	lipgloss.Fprintf(w, "%s %s\n", ui.ErrorBold.Render(ui.SymbolError), ui.Error.Render(msg))

	if hint := clierror.HintOf(err); hint != "" {
		lipgloss.Fprintf(w, "%s%s %s\n", ui.Indent1, ui.Dim.Render(ui.SymbolArrow), hint)
	}
}

// firstErrorClause trims a wrapped chain to its most useful prefix: it keeps
// clauses until one adds no information (pure plumbing like "exit status 1").
//
// If the very first clause is itself plumbing, there is no useful prefix to
// cut to; instead, plumbing clauses are trimmed off the end so any leading
// substantive text survives (and the sentinel tail never leaks through).
func firstErrorClause(msg string) string {
	parts := strings.Split(msg, ": ")
	cut := slices.IndexFunc(parts, plumbingClause)
	if cut < 0 {
		cut = len(parts)
	}

	if cut > 0 {
		if result := strings.Join(parts[:cut], ": "); result != "" {
			return result
		}

		return msg
	}

	end := len(parts)
	for end > 0 && plumbingClause(parts[end-1]) {
		end--
	}

	if end == 0 {
		return msg
	}

	return strings.Join(parts[:end], ": ")
}

// plumbingClause reports whether s is an unhelpful error-chain detail.
func plumbingClause(s string) bool {
	if strings.HasPrefix(s, "exit status ") {
		return true
	}

	switch s {
	case "docker command failed", "invalid input", "not found",
		"forbidden", "internal error", "authentication failed",
		"keychain unavailable", "context canceled":
		return true
	}

	return false
}

// isKnownCommand reports whether name matches a registered command or alias.
func isKnownCommand(root *cobra.Command, name string) bool {
	if found, _, err := root.Find([]string{name}); err == nil && found != nil && found.Name() == name {
		return true
	}

	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return true
		}

		if slices.Contains(cmd.Aliases, name) {
			return true
		}
	}

	return false
}
