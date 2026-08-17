package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// runRoot executes the real command tree, capturing the arguments delivered to
// the named command's RunE. It returns those arguments and any execution error.
func runRoot(t *testing.T, target *cobra.Command, args ...string) ([]string, error) {
	t.Helper()

	original := target.RunE

	var got []string

	target.RunE = func(_ *cobra.Command, a []string) error {
		got = a

		return nil
	}

	var out bytes.Buffer

	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)

	t.Cleanup(func() {
		target.RunE = original
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		resetGlobalFlags(t)
	})

	err := rootCmd.Execute()

	return got, err
}

// resetGlobalFlags restores the root's persistent flags between cases, since
// they are package-level state shared across subtests.
func resetGlobalFlags(t *testing.T) {
	t.Helper()

	for _, name := range []string{"verbose", "no-interaction"} {
		if f := rootCmd.PersistentFlags().Lookup(name); f != nil {
			_ = f.Value.Set("false")
			f.Changed = false
		}
	}

	if f := rootCmd.PersistentFlags().Lookup("config"); f != nil {
		_ = f.Value.Set("")
		f.Changed = false
	}

	viper.Reset()
}

// TestPassthroughForwardsEverythingAfterCommand is the core contract: once a
// passthrough command is named, every remaining token belongs to the target
// tool, including anything that looks like a flag.
func TestPassthroughForwardsEverythingAfterCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "short flag", args: []string{"php", "-v"}, want: []string{"-v"}},
		{name: "long flag", args: []string{"php", "--version"}, want: []string{"--version"}},
		{name: "script path", args: []string{"php", "script.php"}, want: []string{"script.php"}},
		{
			name: "relative script path",
			args: []string{"php", "development/scripts/sync-versions.php"},
			want: []string{"development/scripts/sync-versions.php"},
		},
		{name: "subcommand and flag", args: []string{"php", "outdated", "--direct"}, want: []string{"outdated", "--direct"}},
		{name: "flag before positional", args: []string{"php", "--direct", "outdated"}, want: []string{"--direct", "outdated"}},
		{
			name: "xf global flag is not intercepted",
			args: []string{"php", "--no-interaction", "script.php"},
			want: []string{"--no-interaction", "script.php"},
		},
		{name: "help is forwarded", args: []string{"php", "--help"}, want: []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runRoot(t, phpCmd, tt.args...)
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("forwarded %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGlobalFlagsBeforeCommandAreConsumedByXF covers the other half of the rule:
// xf's own flags work when given before the command name. This is what
// TraverseChildren on the root buys; without it these are forwarded instead.
func TestGlobalFlagsBeforeCommandAreConsumedByXF(t *testing.T) {
	tests := []struct {
		name string
		args []string
		flag string
	}{
		{name: "verbose long", args: []string{"--verbose", "php", "script.php"}, flag: "verbose"},
		{name: "verbose short", args: []string{"-v", "php", "script.php"}, flag: "verbose"},
		{name: "no-interaction long", args: []string{"--no-interaction", "php", "script.php"}, flag: "no-interaction"},
		{name: "no-interaction short", args: []string{"-n", "php", "script.php"}, flag: "no-interaction"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runRoot(t, phpCmd, tt.args...)
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}

			if !reflect.DeepEqual(got, []string{"script.php"}) {
				t.Errorf("forwarded %v, want only the script path", got)
			}

			flag := rootCmd.PersistentFlags().Lookup(tt.flag)
			if flag == nil {
				t.Fatalf("global flag %q not found", tt.flag)
			}

			if flag.Value.String() != "true" {
				t.Errorf("global flag %q was not set on xf", tt.flag)
			}
		})
	}
}

// TestConfigFlagBeforeCommandTakesAValue covers the one global that consumes a
// following argument, which is the case most likely to mis-split.
func TestConfigFlagBeforeCommandTakesAValue(t *testing.T) {
	// cobra.OnInitialize loads this path for real, so it must exist.
	cfgPath := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(cfgPath, []byte("verbose: false\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	for _, flag := range []string{"--config", "-c"} {
		t.Run(flag, func(t *testing.T) {
			got, err := runRoot(t, phpCmd, flag, cfgPath, "php", "script.php")
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}

			if !reflect.DeepEqual(got, []string{"script.php"}) {
				t.Errorf("forwarded %v, want only the script path", got)
			}

			if f := rootCmd.PersistentFlags().Lookup("config"); f.Value.String() != cfgPath {
				t.Errorf("config = %q, want %q", f.Value.String(), cfgPath)
			}
		})
	}
}

// TestFlagSeparatorStillAccepted ensures the explicit `--` form keeps working.
// With DisableFlagParsing cobra no longer strips it, so stripFlagSeparator is
// load-bearing rather than vestigial.
func TestFlagSeparatorStillAccepted(t *testing.T) {
	got, err := runRoot(t, phpCmd, "php", "--", "-v")
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}

	if !reflect.DeepEqual(got, []string{"--", "-v"}) {
		t.Fatalf("RunE received %v, want the separator to reach it untouched", got)
	}

	if stripped := stripFlagSeparator(got); !reflect.DeepEqual(stripped, []string{"-v"}) {
		t.Errorf("stripFlagSeparator(%v) = %v, want [-v]", got, stripped)
	}
}

// TestEveryPassthroughCommandDisablesFlagParsing guards against a new
// passthrough command being added without the setting.
func TestEveryPassthroughCommandDisablesFlagParsing(t *testing.T) {
	for _, cmd := range []*cobra.Command{phpCmd, phpDebugCmd, composerCmd, composeCmd, execCmd, debugCmd} {
		t.Run(cmd.Name(), func(t *testing.T) {
			if !cmd.DisableFlagParsing {
				t.Error("passthrough command does not set DisableFlagParsing")
			}
		})
	}
}

// TestRootTraversesChildren pins the setting that keeps pre-command globals
// working. Removing it silently breaks `xf --verbose php ...`.
func TestRootTraversesChildren(t *testing.T) {
	if !rootCmd.TraverseChildren {
		t.Error("rootCmd must set TraverseChildren for pre-command global flags to work")
	}
}

// TestNonPassthroughCommandsKeepFlexibleFlagPositions covers the requirement
// that ordinary commands still accept their flags in any position, before or
// after positional arguments. Only passthrough commands take a strict rule.
func TestNonPassthroughCommandsKeepFlexibleFlagPositions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "flag before positional", args: []string{"logs", "--follow", "xf"}, want: []string{"xf"}},
		{name: "flag after positional", args: []string{"logs", "xf", "--follow"}, want: []string{"xf"}},
		{name: "flag interspersed", args: []string{"logs", "xf", "--follow", "web"}, want: []string{"xf", "web"}},
		{name: "global after positional", args: []string{"logs", "xf", "--verbose"}, want: []string{"xf"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := runRoot(t, logsCmd, tt.args...)
			if err != nil {
				t.Fatalf("execution failed: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("positional args = %v, want %v", got, tt.want)
			}

			t.Cleanup(func() {
				if f := logsCmd.Flags().Lookup("follow"); f != nil {
					_ = f.Value.Set("false")
					f.Changed = false
				}
			})
		})
	}
}

// TestLogsIsNotAPassthroughCommand documents a deliberate decision: --follow
// belongs to xf, so logs keeps normal parsing.
func TestLogsIsNotAPassthroughCommand(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"logs"})
	if err != nil {
		t.Fatalf("could not find logs: %v", err)
	}

	if cmd.DisableFlagParsing {
		t.Error("logs must keep normal flag parsing; --follow is an xf flag")
	}
}
