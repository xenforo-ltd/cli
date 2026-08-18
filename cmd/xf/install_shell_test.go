package main

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestShellQuoteNeutralisesShellSyntax(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"command separator", "x; touch /tmp/pwned"},
		{"command substitution", "x$(touch /tmp/pwned)"},
		{"backticks", "x`touch /tmp/pwned`"},
		{"pipe", "x | touch /tmp/pwned"},
		{"glob", "*"},
		{"single quote", "it's"},
		{"newline", "x\ntouch /tmp/pwned"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Round-trip through a real shell: echo must reproduce the input
			// exactly, which is only true if nothing was interpreted.
			script := "printf %s " + shellQuote(tc.in)

			out, err := exec.CommandContext(context.Background(), "sh", "-c", script).Output()
			if err != nil {
				t.Fatalf("sh -c failed: %v", err)
			}

			if string(out) != tc.in {
				t.Errorf("shell interpreted the value: got %q, want %q", string(out), tc.in)
			}
		})
	}
}

func TestInstallShellCommandQuotesInstallerValues(t *testing.T) {
	command := installShellCommand([]string{
		"xf:install",
		"--title=Chris' Forum; touch /tmp/pwned",
	})

	// The dangerous value must be quoted, so the separator cannot terminate
	// the installer command.
	if strings.Contains(command, "; touch /tmp/pwned'") == false {
		t.Errorf("value was not quoted as a single word: %s", command)
	}

	if strings.HasSuffix(command, "touch /tmp/pwned") {
		t.Errorf("command ends with an unquoted injection: %s", command)
	}
}

func TestInstallShellCommandKeepsThePasswordOutOfArgv(t *testing.T) {
	command := installShellCommand([]string{"xf:install"})

	// The password must be read from the environment at run time, and the
	// substitution must be quoted so spaces and globs survive intact.
	if !strings.Contains(command, `--password="$XF_INSTALL_PASSWORD"`) {
		t.Errorf("password is not read from the environment: %s", command)
	}
}

func TestInstallShellCommandPassesAwkwardPasswordsVerbatim(t *testing.T) {
	command := installShellCommand([]string{"xf:install"})

	// Run the built command with a stand-in for php so the installer's view
	// of the password can be observed. printf %s\n prints each argument on
	// its own line, so a split password would show up as extra lines.
	script := strings.Replace(command, `'php' 'cmd.php'`, `printf '%s\n'`, 1)
	if script == command {
		t.Fatalf("could not substitute the interpreter in %q", command)
	}

	cmd := exec.CommandContext(context.Background(), "sh", "-c", script)
	cmd.Env = append(cmd.Environ(), "XF_INSTALL_PASSWORD=a b * c'd")

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sh -c failed: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")

	last := lines[len(lines)-1]
	if last != `--password=a b * c'd` {
		t.Errorf("password reached the installer as %q", last)
	}
}

// Command substitution strips trailing newlines, so the password must be
// expanded directly: a password ending in one would otherwise reach the
// installer altered.
func TestInstallShellCommandPreservesTrailingNewlinesInThePassword(t *testing.T) {
	command := installShellCommand([]string{"xf:install"})

	script := strings.Replace(command, `'php' 'cmd.php'`, `printf '%s'`, 1)
	if script == command {
		t.Fatalf("could not substitute the interpreter in %q", command)
	}

	cmd := exec.CommandContext(context.Background(), "sh", "-c", script)
	cmd.Env = append(cmd.Environ(), "XF_INSTALL_PASSWORD=secret\n")

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sh -c failed: %v", err)
	}

	if !strings.HasSuffix(string(out), "--password=secret\n") {
		t.Errorf("trailing newline lost: output ended %q", string(out))
	}
}
