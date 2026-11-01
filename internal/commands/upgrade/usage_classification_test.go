package upgrade

import (
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/xf"
)

func TestInvocationErrorsAreUsageErrors(t *testing.T) {
	t.Parallel()

	t.Run("upgrade missing non-interactive flags", func(t *testing.T) {
		t.Parallel()

		err := validateUpgradeFlags(&UpgradeOptions{})
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

	t.Run("upgrade target version not newer", func(t *testing.T) {
		t.Parallel()

		opts := &UpgradeOptions{
			TargetVersionID: 100,
			currentVersion:  &xf.Version{ID: 200},
		}

		err := executeUpgrade(t.Context(), opts)
		if err == nil {
			t.Fatal("expected an error")
		}

		if clierror.IsUsage(err) {
			t.Error("a version-ordering failure is not an invocation mistake; usage should not print")
		}
	})
}
