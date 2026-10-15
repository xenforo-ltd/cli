package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/xenforo-ltd/cli/internal/devoutput"
)

// exportDataTask generates release XML from prepared development output.
type exportDataTask struct {
	dir    string
	stderr io.Writer
}

func (t exportDataTask) run(ctx context.Context, root *os.Root) error {
	result, err := devoutput.Export(ctx, filepath.Join(root.Name(), filepath.FromSlash(t.dir)))
	if err != nil {
		return fmt.Errorf("exporting add-on data: %w", err)
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintln(t.stderr, "warning: "+warning); err != nil {
			return fmt.Errorf("writing export warning: %w", err)
		}
	}
	return nil
}
