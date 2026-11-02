package clierror_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"charm.land/huh/v2"

	"github.com/xenforo-ltd/cli/internal/clierror"
)

func TestErrorAnnotationsSurviveWrapping(t *testing.T) {
	cause := errors.New("invalid port")
	classified := clierror.MarkAs(clierror.ErrInvalidInput, "configure database: %w", cause)
	if clierror.IsUsage(classified) {
		t.Fatal("invalid input alone must not request usage")
	}
	if got := clierror.HintOf(classified); got != "" {
		t.Fatalf("unhinted error has hint %q", got)
	}
	err := fmt.Errorf("initialization failed: %w", clierror.WithHint(
		clierror.Usage(classified), "Use a port from 1 to 65535"))
	for _, target := range []error{cause, clierror.ErrInvalidInput} {
		if !errors.Is(err, target) {
			t.Errorf("error lost classification or cause %v: %v", target, err)
		}
	}
	if !clierror.IsUsage(err) {
		t.Error("wrapped invocation error must request usage")
	}
	if got := clierror.HintOf(err); got != "Use a port from 1 to 65535" {
		t.Errorf("hint = %q", got)
	}
	if got, want := err.Error(), "initialization failed: configure database: invalid port"; got != want {
		t.Errorf("message = %q, want %q; annotations must not add sentinel text", got, want)
	}
}

func TestPromptDistinguishesInterruptFromFailure(t *testing.T) {
	t.Run("user interrupt", func(t *testing.T) {
		err := clierror.Prompt(fmt.Errorf("form: %w", huh.ErrUserAborted), "select %s", "license")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("prompt interrupt must be recognizable as cancellation: %v", err)
		}
		if errors.Is(err, clierror.ErrCancelled) {
			t.Fatal("interrupt must not become a successful menu cancellation")
		}
	})
	t.Run("input failure", func(t *testing.T) {
		err := clierror.Prompt(io.ErrUnexpectedEOF, "select %s", "license")
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("prompt lost input error: %v", err)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, clierror.ErrCancelled) {
			t.Fatalf("input failure must not be treated as cancellation: %v", err)
		}
	})
}

func TestPassthroughPreservesLaunchFailure(t *testing.T) {
	if err := clierror.Passthrough(nil, "run PHP"); err != nil {
		t.Fatalf("successful process returned %v", err)
	}
	cause := errors.New("executable not found")
	err := clierror.Passthrough(cause, "run PHP")
	if !errors.Is(err, cause) {
		t.Fatalf("launch failure lost its cause: %v", err)
	}
	if code, ok := clierror.ExitCode(err); ok {
		t.Fatalf("launch failure became child exit status %d", code)
	}
	if got := err.Error(); got != "run PHP: executable not found" {
		t.Errorf("launch failure lacks operation context: %q", got)
	}
	wrapped := fmt.Errorf("command failed: %w", clierror.WithExitCode(23))
	if code, ok := clierror.ExitCode(wrapped); !ok || code != 23 {
		t.Errorf("wrapped child exit status = (%d, %t), want (23, true)", code, ok)
	}
}
