package setup

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

func PrintSkippedStep(current, total int, label, reason string) {
	ui.Printf("%s %s %s\n", ui.Step(current, total), ui.Bold.Render(label), ui.Dim.Render("(skipped: "+reason+")"))
}

func PrintStartHint(dir string) {
	ui.PrintHint("Run " + ui.Command.Render("xf up") + " in " + ui.Path.Render(ui.ShortHome(dir)) + " to start the environment")
}

// InstanceTitle formats a site title with its instance name. An empty title
// defaults to "XenForo".
func InstanceTitle(title, instanceName string) string {
	if title == "" {
		title = "XenForo"
	}

	return fmt.Sprintf("%s [%s]", title, instanceName)
}

func ChooseBoardURL(instanceName, detectedURL string, detectedErr error) (string, bool) {
	if detectedErr != nil || strings.TrimSpace(detectedURL) == "" {
		return environment.FallbackBoardURL(instanceName), false
	}

	return detectedURL, true
}

// installShellCommand builds the shell command that runs xf:install.
//
// Every argument is shell-quoted, so installer values such as the site title
// cannot inject shell syntax. The password substitution is quoted too, so a
// password containing spaces or glob characters reaches the installer
// verbatim.
//
// The password is passed through the environment and expanded by sh rather
// than being interpolated here. That keeps it out of xf's own argv, out of the
// docker compose invocation, and out of anything that logs either of those.
//
// It does not keep it out of the php process's argv inside the container:
// XF\Cli\Command\Install accepts the administrator password only via
// --password or an interactive hidden prompt, and --no-interaction rules the
// prompt out. So for the lifetime of the install, the password is visible to
// anything that can list processes in that container. The container is a
// single-tenant development environment created by this tool, so that exposure
// is accepted; it should be revisited if XenForo ever accepts the password on
// stdin or from an environment variable of its own.
func installShellCommand(installArgs []string) string {
	command := shellJoinArgs(append([]string{"php", "cmd.php"}, installArgs...))
	// Expanded directly rather than through $(printenv ...): command
	// substitution strips trailing newlines, so a password ending in one would
	// reach the installer altered.
	return command + ` --password="$XF_INSTALL_PASSWORD"`
}

// InstallInvocation returns the sh arguments and environment that run xf:install.
func InstallInvocation(user, email, title, siteURL, password string) ([]string, map[string]string) {
	installArgs := []string{
		"xf:install",
		"--no-interaction",
		"--clear",
		"--user=" + user,
		"--email=" + email,
		"--title=" + title,
		"--url=" + siteURL,
	}

	return []string{"sh", "-c", installShellCommand(installArgs)}, map[string]string{"XF_INSTALL_PASSWORD": password}
}

func shellJoinArgs(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = shellQuote(arg)
	}

	return strings.Join(parts, " ")
}

// shellQuote renders a string as a single-quoted POSIX shell word.
//
// Every argument is quoted unconditionally. Quoting only those that look
// dangerous is how injection gets through: a value such as
// `--title=x;rm -rf /` contains no spaces or quotes, so a
// looks-dangerous test passes it to the shell verbatim.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func PrintUsefulCommands() {
	commands := []ui.KVPair{
		ui.KV("xf up", "Start the environment"),
		ui.KV("xf down", "Stop the environment"),
		ui.KV("xf reboot", "Restart the environment"),
		ui.KV("xf ps", "Container status"),
		ui.KV("xf logs", "Show logs"),
		ui.KV("xf composer", "Run Composer"),
		ui.KV("xf php", "Run PHP"),
	}

	width := 0
	for _, c := range commands {
		if w := lipgloss.Width(c.Key); w > width {
			width = w
		}
	}

	ui.Println(ui.Bold.Render("Useful commands:"))
	for _, c := range commands {
		ui.Printf("%s%s  %s\n", ui.Indent1, ui.Command.Render(padRight(c.Key, width)), ui.Muted.Render(c.Value))
	}
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}

	return s + strings.Repeat(" ", width-w)
}
