package main

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

func TestPassthroughCommandsForwardFlags(t *testing.T) {
	commands := []*cobra.Command{composerCmd, phpCmd, phpDebugCmd, composeCmd, execCmd, debugCmd}
	want := []string{"outdated", "--direct"}
	for _, cmd := range commands {
		t.Run(cmd.Name(), func(t *testing.T) {
			if !cmd.DisableFlagParsing {
				t.Fatal("expected a passthrough command to set DisableFlagParsing")
			}
			got := captureForwardedArgs(t, cmd, want...)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("args = %v, want %v", got, want)
			}
		})
	}
}

func captureForwardedArgs(t *testing.T, cmd *cobra.Command, args ...string) []string {
	t.Helper()
	original := cmd.RunE
	var got []string
	cmd.RunE = func(_ *cobra.Command, a []string) error { got = a; return nil }
	t.Cleanup(func() { cmd.RunE = original; rootCmd.SetArgs(nil); rootCmd.SetOut(nil); rootCmd.SetErr(nil) })
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{cmd.Name()}, args...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("executing %s failed: %v", cmd.Name(), err)
	}
	return got
}
