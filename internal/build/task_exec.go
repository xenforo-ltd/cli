package build

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// execTask runs configured argument vectors in the add-on directory without a shell.
type execTask struct {
	commands [][]string
	dir      string
	streams  commandStreams
}

// run executes commands in declaration order and stops at the first error.
func (t execTask) run(ctx context.Context, root *os.Root) error {
	for _, command := range t.commands {
		if err := runCommand(ctx, root, t.streams, t.dir, command[0], command[1:]...); err != nil {
			return fmt.Errorf("exec %s: %w", strings.Join(command, " "), err)
		}
	}

	return nil
}
