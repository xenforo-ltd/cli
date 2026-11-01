package exec

import (
	"errors"
	"testing"

	"github.com/xenforo-ltd/cli/internal/clierror"
)

func TestInvocationErrorsAreUsageErrors(t *testing.T) {
	t.Parallel()

	t.Run("exec with too few arguments", func(t *testing.T) {
		t.Parallel()

		err := validateExecInvocation([]string{"only-service"})
		if err == nil {
			t.Fatal("expected an error")
		}

		if !clierror.IsUsage(err) {
			t.Errorf("expected a usage error, got %T: %v", err, err)
		}

		if !errors.Is(err, clierror.ErrInvalidInput) {
			t.Error("expected the error to still wrap ErrInvalidInput")
		}
	})
}
