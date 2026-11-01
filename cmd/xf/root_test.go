package main

import (
	"sync/atomic"
	"syscall"
	"testing"
)

func TestCommandsAreGrouped(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		if c.GroupID == "" {
			t.Errorf("command %q has no GroupID", c.Name())
		}
	}
}

func TestFirstErrorClause(t *testing.T) {
	in := "failed to start Docker environment: docker command failed: exit status 1"
	if got := firstErrorClause(in); got != "failed to start Docker environment" {
		t.Errorf("got %q", got)
	}
	if got := firstErrorClause("plain message"); got != "plain message" {
		t.Errorf("got %q", got)
	}
}

func TestFirstErrorClauseLeadingPlumbing(t *testing.T) {
	in := "docker command failed: container xf-web-1 is unhealthy: exit status 1"
	if got := firstErrorClause(in); got != "docker command failed: container xf-web-1 is unhealthy" {
		t.Errorf("got %q", got)
	}
	allPlumbing := "docker command failed: not found"
	if got := firstErrorClause(allPlumbing); got == "" {
		t.Errorf("firstErrorClause must never return empty, got %q", got)
	}
	leadingEmpty := ": exit status 1"
	if got := firstErrorClause(leadingEmpty); got == "" {
		t.Errorf("firstErrorClause(%q) returned empty", leadingEmpty)
	}
}

func TestIsKnownCommandIncludesCobraCompletion(t *testing.T) {
	if !isKnownCommand("completion") {
		t.Fatal("expected completion to be treated as a known command")
	}
}

func TestInterruptExitCodeFollowsTheSignal(t *testing.T) {
	t.Cleanup(func() { interruptSignal = atomic.Value{} })
	if got := interruptExitCode(); got != exitInterrupted {
		t.Errorf("with no signal recorded, got %d, want %d", got, exitInterrupted)
	}
	recordInterruptSignal(syscall.SIGINT)
	if got := interruptExitCode(); got != 130 {
		t.Errorf("after SIGINT, got %d, want 130", got)
	}
	recordInterruptSignal(syscall.SIGTERM)
	if got := interruptExitCode(); got != 143 {
		t.Errorf("after SIGTERM, got %d, want 143", got)
	}
}
