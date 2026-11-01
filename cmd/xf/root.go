package main

import (
	"context"
	"errors"
	"fmt"
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
	// Without a Run, cobra answers unknown positional arguments with the root
	// help and exit 0. That happens when a global flag precedes a forwarded
	// XenForo command (`xf -v xf-dev:import`): execute only takes the direct
	// route when the first argument is not a flag, so cobra receives the
	// command name and finds no subcommand. Fail loudly instead, so scripts
	// do not see a false success.
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

// configureErrorHandling makes cobra print usage only for genuine misuse.
//
// By default cobra prints the full usage block for any error returned from RunE,
// including runtime failures such as Docker being unavailable. That wrongly
// implies the command was typed incorrectly. Usage is still printed for argument
// and flag errors, where it is the helpful response.
func configureErrorHandling(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	// execute prints errors itself; without this cobra prints them too.
	cmd.SilenceErrors = true

	// execute may be called more than once by tests or an embedding program. Do
	// not stack another usageError wrapper on every invocation.
	if cmd.Args != nil && (cmd.Annotations[usageConfiguredAnnotation] != "true") {
		args := cmd.Args
		cmd.Args = func(c *cobra.Command, a []string) error {
			if err := args(c, a); err != nil {
				return clierror.Usage(err)
			}

			return nil
		}
	}
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	cmd.Annotations[usageConfiguredAnnotation] = "true"

	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clierror.Usage(err)
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

// execute runs the CLI application.
func execute(ctx context.Context) {
	configureErrorHandling(rootCmd)

	if len(os.Args) > 1 && takesDirectXenForoRoute(os.Args[1]) && !isKnownCommand(os.Args[1]) {
		if err := rootcommand.Run(ctx, os.Args[1:]); err != nil {
			exitForQuietErrors(err)
			handleError(err)
			os.Exit(1)
		}

		return
	}

	executed, err := rootCmd.ExecuteContextC(ctx)
	if err != nil {
		exitForQuietErrors(err)
		handleError(err)

		if clierror.IsUsage(err) && executed != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprint(os.Stderr, executed.UsageString())
		}

		os.Exit(1)
	}
}

// exitForQuietErrors exits without reporting when err is not a failure to
// show: an interrupt, a cancellation, or a child's own exit status.
//
// Ctrl-C is a deliberate user action, so it exits with the conventional signal
// status rather than reporting an error.
func exitForQuietErrors(err error) {
	if isInterrupted(err) {
		os.Exit(interruptExitCode())
	}

	if errors.Is(err, clierror.ErrCancelled) {
		os.Exit(0)
	}

	if code, ok := clierror.ExitCode(err); ok {
		os.Exit(code)
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
	return errors.Is(err, context.Canceled)
}

// takesDirectXenForoRoute reports whether a first argument is eligible to be
// forwarded straight to XenForo, bypassing cobra.
//
// Known limitation: a leading flag is not eligible, so a global flag cannot be
// combined with a direct XenForo command. `xf -v xf-dev:import` falls through to
// cobra, where rootCmd's RunE rejects it as an unknown command.
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

	if hint := clierror.HintOf(err); hint != "" {
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

func init() {
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
	// lipgloss's writer-side profile detection and piped/redirected help
	// (e.g. `xf --help | cat`) would carry raw escape codes. Downsample through
	// lipgloss.Sprint, which applies the same stdout profile detection as
	// every other styled line (and the stderr error renderer), so help and
	// errors agree on when to style.
	cobra.AddTemplateFunc("styleHeading", func(s string) string {
		return lipgloss.Sprint(ui.Bold.Render(s))
	})
	cobra.AddTemplateFunc("styleCommand", func(s string) string {
		return lipgloss.Sprint(ui.Command.Render(s))
	})
	rootCmd.SetUsageTemplate(usageTemplate)

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "path to config file")
	rootCmd.PersistentFlags().BoolP("no-interaction", "n", false, "disable interactive prompts")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")

	cobra.CheckErr(viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose")))
	cobra.CheckErr(viper.BindPFlag("no_interaction", rootCmd.PersistentFlags().Lookup("no-interaction")))
}
