package main

import "testing"

// TestDBCmdAcceptsOnlyOnePath matches the [path] argument the help advertises.
func TestDBCmdAcceptsOnlyOnePath(t *testing.T) {
	cmd := findCommand(t, "db")

	if cmd.Args == nil {
		t.Fatal("db command has no Args validator")
	}

	if err := cmd.Args(cmd, []string{"one", "two"}); err == nil {
		t.Fatal("expected an error for more than one path")
	}
}
