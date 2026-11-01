package initialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierror"
)

func TestInvocationErrorsAreUsageErrors(t *testing.T) {
	t.Parallel()

	t.Run("init missing non-interactive flags", func(t *testing.T) {
		t.Parallel()

		err := validateNonInteractiveFlags(&InitOptions{})
		if err == nil {
			t.Fatal("expected an error")
		}

		if !clierror.IsUsage(err) {
			t.Errorf("expected a usage error, got %T: %v", err, err)
		}
	})
}

func TestNonInvocationErrorsAreNotUsageErrors(t *testing.T) {
	t.Parallel()

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

		if clierror.IsUsage(err) {
			t.Error("an environment-state failure is not an invocation mistake; usage should not print")
		}
	})
}
