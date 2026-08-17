package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/xenforo-ltd/cli/internal/xf"
)

func isUsageError(err error) bool {
	var usageErr *usageError

	return errors.As(err, &usageErr)
}

// TestInvocationErrorsAreUsageErrors covers the sites audited as genuine
// invocation mistakes, where printing usage helps the user correct the command.
func TestInvocationErrorsAreUsageErrors(t *testing.T) {
	t.Parallel()

	t.Run("exec with too few arguments", func(t *testing.T) {
		t.Parallel()

		err := validateExecInvocation([]string{"only-service"})
		if err == nil {
			t.Fatal("expected an error")
		}

		if !isUsageError(err) {
			t.Errorf("expected a usage error, got %T: %v", err, err)
		}

		if !errors.Is(err, ErrInvalidInput) {
			t.Error("expected the error to still wrap ErrInvalidInput")
		}
	})

	t.Run("init missing non-interactive flags", func(t *testing.T) {
		t.Parallel()

		err := validateNonInteractiveFlags(&InitOptions{})
		if err == nil {
			t.Fatal("expected an error")
		}

		if !isUsageError(err) {
			t.Errorf("expected a usage error, got %T: %v", err, err)
		}
	})

	t.Run("upgrade missing non-interactive flags", func(t *testing.T) {
		t.Parallel()

		err := validateUpgradeFlags(&UpgradeOptions{})
		if err == nil {
			t.Fatal("expected an error")
		}

		if !isUsageError(err) {
			t.Errorf("expected a usage error, got %T: %v", err, err)
		}
	})
}

// TestNonInvocationErrorsAreNotUsageErrors is the more important half of the
// audit. These errors also wrap ErrInvalidInput, but they do not mean the
// command was typed incorrectly, so printing usage would be noise or actively
// misleading. A blanket rule keyed on ErrInvalidInput would wrongly catch them.
func TestNonInvocationErrorsAreNotUsageErrors(t *testing.T) {
	t.Parallel()

	t.Run("upgrade target version not newer", func(t *testing.T) {
		t.Parallel()

		opts := &UpgradeOptions{
			TargetVersionID: 100,
			CurrentVersion:  &xf.Version{ID: 200},
		}

		err := executeUpgrade(t.Context(), opts)
		if err == nil {
			t.Fatal("expected an error")
		}

		if isUsageError(err) {
			t.Error("a version-ordering failure is not an invocation mistake; usage should not print")
		}
	})

	t.Run("init target path is not a directory", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(t.TempDir(), "a-file")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}

		err := prepareTargetDirectory(file)
		if err == nil {
			t.Fatal("expected an error")
		}

		if isUsageError(err) {
			t.Error("an environment-state failure is not an invocation mistake; usage should not print")
		}
	})
}

// TestCancellationIsNotAUsageError guards the case that most clearly must not
// print usage: the user deliberately cancelled an interactive flow.
//
// The cancellation error is constructed inline in runInteractiveReview's "cancel"
// branch, which needs a terminal to reach. This asserts the property that branch
// relies on: an ErrInvalidInput error is not a usage error unless explicitly
// classified as one, so cancelling never triggers a usage dump.
func TestCancellationIsNotAUsageError(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("initialization cancelled: %w", ErrInvalidInput)

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatal("expected the sentinel to be wrapped")
	}

	if isUsageError(err) {
		t.Error("cancelling an interactive flow must not print usage")
	}
}
