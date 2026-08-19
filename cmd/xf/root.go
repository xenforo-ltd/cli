package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/version"
	"github.com/xenforo-ltd/cli/internal/xf"
)

var configFile string

var rootCmd = &cobra.Command{
	Use:     "xf",
	Version: version.Version,
	// Required by the passthrough commands, which set DisableFlagParsing so that
	// everything after them reaches the target tool. Without TraverseChildren,
	// cobra defers parsing the root's persistent flags to the subcommand, which
	// then forwards them instead: even `xf --verbose php script.php` would be
	// swallowed, leaving no way to set xf's own flags on those commands.
	TraverseChildren: true,
	Short:            "Provision and manage XenForo development environments",
	Long: `Provision and manage Docker-based XenForo development environments:
authentication, package downloads, caching, containers and worktrees.

Inside a XenForo directory, unknown commands are forwarded to XenForo itself,
so ` + "`xf list`" + ` runs ` + "`cmd.php list`" + ` in the environment.`,
	Example: `  # Authenticate, then create a project
  xf auth login
  xf init ./my-project

  # Run a XenForo command in the environment
  xf xf-dev:import`,
}

// usageError marks an error as caused by incorrect invocation (bad arguments or
// flags), for which printing usage is genuinely helpful.
type usageError struct {
	err error
}

func newUsageError(err error) error {
	return &usageError{err: err}
}

func (e *usageError) Error() string { return e.err.Error() }

func (e *usageError) Unwrap() error { return e.err }

// configureErrorHandling makes cobra print usage only for genuine misuse.
//
// By default cobra prints the full usage block for any error returned from RunE,
// including runtime failures such as Docker being unavailable. That wrongly
// implies the command was typed incorrectly. Usage is still printed for argument
// and flag errors, where it is the helpful response.
func configureErrorHandling(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	// Execute prints errors itself; without this cobra prints them too.
	cmd.SilenceErrors = true

	// Execute may be called more than once by tests or an embedding program. Do
	// not stack another usageError wrapper on every invocation.
	if cmd.Args != nil && (cmd.Annotations == nil || cmd.Annotations[usageConfiguredAnnotation] != "true") {
		args := cmd.Args
		cmd.Args = func(c *cobra.Command, a []string) error {
			if err := args(c, a); err != nil {
				return newUsageError(err)
			}

			return nil
		}
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[usageConfiguredAnnotation] = "true"

	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return newUsageError(err)
	})

	for _, sub := range cmd.Commands() {
		configureErrorHandling(sub)
	}
}

const usageConfiguredAnnotation = "xf.xenforo.com/usage-error-handling-configured"

// usageTemplate is cobra's default usage template with the section headings
// and command names styled via the styleHeading/styleCommand template funcs
// registered in init(). It is not otherwise restructured.
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

// Execute runs the CLI application.
func Execute(ctx context.Context) {
	configureErrorHandling(rootCmd)

	if len(os.Args) > 1 {
		firstArg := os.Args[1]

		if takesDirectXenForoRoute(firstArg) {
			if !isKnownCommand(firstArg) {
				if err := runAsXenForoCommand(ctx, os.Args[1:], exec.CommandContext); err != nil {
					if isInterrupted(err) {
						os.Exit(interruptExitCode())
					}

					if errors.Is(err, ErrCancelled) {
						os.Exit(0)
					}
					var exitErr *exitCodeError
					if errors.As(err, &exitErr) {
						os.Exit(exitErr.code)
					}

					handleError(err)
					os.Exit(1)
				}

				return
			}
		}
	}

	executed, err := rootCmd.ExecuteContextC(ctx)
	if err != nil {
		// Ctrl-C is a deliberate user action, not a failure. Exit quietly with
		// the conventional signal status rather than reporting an error.
		if isInterrupted(err) {
			os.Exit(interruptExitCode())
		}

		if errors.Is(err, ErrCancelled) {
			os.Exit(0)
		}
		var exitErr *exitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.code)
		}

		handleError(err)

		var usageErr *usageError
		if errors.As(err, &usageErr) && executed != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprint(os.Stderr, executed.UsageString())
		}

		os.Exit(1)
	}
}

// exitInterrupted is the conventional exit status for a process terminated by
// SIGINT (128 + 2). It is the default when no signal was recorded, because
// Ctrl-C is the interruption users actually produce.
const exitInterrupted = 130

// interruptSignal records which signal ended the process, so the exit status
// can follow the 128+signum convention rather than always reporting SIGINT.
var interruptSignal atomic.Value

// recordInterruptSignal stores the signal that terminated the process.
func recordInterruptSignal(sig os.Signal) {
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
	return err != nil && errors.Is(err, context.Canceled)
}

// takesDirectXenForoRoute reports whether a first argument is eligible to be
// forwarded straight to XenForo, bypassing cobra.
//
// Known limitation: a leading flag is not eligible, so a global flag cannot be
// combined with a direct XenForo command. `xf -v xf-dev:import` falls through to
// cobra, which resolves nothing and prints the root help without an error.
func takesDirectXenForoRoute(firstArg string) bool {
	return !strings.HasPrefix(firstArg, "-") &&
		firstArg != "help" && firstArg != "--help" && firstArg != "-h"
}

func handleError(err error) {
	msg := err.Error()
	if !viper.GetBool("verbose") {
		msg = firstErrorClause(msg)
	}

	// Errors belong on stderr, not lipgloss's default stdout writer, so
	// render through lipgloss.Fprintf: like ui's own Print* helpers, it
	// downsamples any ANSI in the rendered text (including from hints built
	// with ui.Command.Render elsewhere) based on stderr's own profile - a
	// plain os.Stderr write would carry the raw escapes straight through
	// regardless of NO_COLOR or piping.
	lipgloss.Fprintf(os.Stderr, "%s %s\n", ui.ErrorBold.Render(ui.SymbolError), ui.Error.Render(msg))

	if hint := hintOf(err); hint != "" {
		lipgloss.Fprintf(os.Stderr, "%s%s %s\n", ui.Indent1, ui.Dim.Render(ui.SymbolArrow), hint)
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
	cut := len(parts)
	for i, p := range parts {
		if plumbingClause(p) {
			cut = i
			break
		}
	}
	if cut > 0 {
		return strings.Join(parts[:cut], ": ")
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

func isKnownCommand(name string) bool {
	if found, _, err := rootCmd.Find([]string{name}); err == nil && found != nil && found.Name() == name {
		return true
	}

	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == name {
			return true
		}

		if slices.Contains(cmd.Aliases, name) {
			return true
		}
	}

	return false
}

// commandFunc builds an external command. It is a seam for tests, and takes a
// context so that cancellation reaches the child process.
type commandFunc func(ctx context.Context, name string, args ...string) *exec.Cmd

func runAsXenForoCommand(ctx context.Context, args []string, cmdFn commandFunc) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine the current directory: %w", err)
	}

	xfDir, err := xf.GetXenForoDir(cwd)
	if err != nil {
		return withHint(
			markAs(err, "unknown command: %s (not in a XenForo directory)", args[0]),
			"Run "+ui.Command.Render("xf --help")+" to see available commands",
		)
	}

	runner, err := dockercompose.NewRunner(xfDir)
	if err != nil {
		if errors.Is(err, dockercompose.ErrEnvNotInitialized) {
			return runAsLocalXenForoCommand(ctx, xfDir, args, cmdFn)
		}

		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	if err := runner.XFCommand(ctx, args...); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return newExitCodeError(ee.ExitCode())
		}

		return fmt.Errorf("failed to run XenForo command %q: %w", args[0], err)
	}

	return nil
}

func runAsLocalXenForoCommand(ctx context.Context, xfDir string, args []string, cmdFn commandFunc) error {
	cmdArgs := append([]string{"cmd.php"}, args...)
	cmd := cmdFn(ctx, "php", cmdArgs...)
	cmd.Dir = xfDir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// The child is killed when the context ends, so report why it stopped
		// rather than the resulting signal.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if errors.Is(err, exec.ErrNotFound) {
			return withHint(
				&kindError{err: errors.New("PHP is not installed or not in your PATH"), kind: err},
				"Install PHP or run this inside a started environment ("+ui.Command.Render("xf up")+")",
			)
		}

		return fmt.Errorf("local XenForo command failed: %w", err)
	}

	return nil
}

func init() {
	cobra.OnInitialize(func() {
		if err := config.Init(configFile); err != nil {
			if errors.As(err, &viper.ConfigFileNotFoundError{}) {
				return
			}

			cobra.CheckErr(err)
		}
	})

	rootCmd.AddGroup(
		&cobra.Group{ID: "start", Title: "Getting started:"},
		&cobra.Group{ID: "env", Title: "Environment:"},
		&cobra.Group{ID: "run", Title: "Run tools:"},
		&cobra.Group{ID: "maint", Title: "Maintenance:"},
	)
	rootCmd.SetHelpCommandGroupID("maint")
	rootCmd.SetCompletionCommandGroupID("maint")

	// Must run after SetCompletionCommandGroupID: it creates the completion
	// command immediately, reading completionCommandGroupID at that point.
	rootCmd.InitDefaultCompletionCmd()

	// Help is written to stdout via fmt, not lipgloss, so it bypasses
	// lipgloss's writer-side profile detection: style only when stdout is an
	// interactive terminal and NO_COLOR is unset, or piped/redirected help
	// (e.g. `xf --help | cat`) would carry raw escape codes.
	cobra.AddTemplateFunc("styleHeading", func(s string) string {
		if !ui.Enabled(os.Stdout) {
			return s
		}
		return ui.Bold.Render(s)
	})
	cobra.AddTemplateFunc("styleCommand", func(s string) string {
		if !ui.Enabled(os.Stdout) {
			return s
		}
		return ui.Command.Render(s)
	})
	rootCmd.SetUsageTemplate(usageTemplate)

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "path to config file")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "disable interactive prompts")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")

	if err := viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")); err != nil {
		cobra.CheckErr(err)
	}

	if err := viper.BindPFlag("no_interaction", rootCmd.PersistentFlags().Lookup("no-interaction")); err != nil {
		cobra.CheckErr(err)
	}
}
