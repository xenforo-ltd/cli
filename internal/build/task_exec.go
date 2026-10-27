package build

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/xenforo-ltd/cli/internal/xfcmd"
)

// execTask runs configured argument vectors in the add-on environment without
// a shell.
type execTask struct {
	addOnPaths
	commands        [][]string
	installationDir string
	streams         commandStreams
}

// run executes commands in declaration order and stops at the first error.
func (t execTask) run(ctx context.Context, root *os.Root) error {
	for _, command := range t.commands {
		if err := t.runCommand(ctx, root, command); err != nil {
			return fmt.Errorf("exec %s: %w", strings.Join(command, " "), err)
		}
	}

	return nil
}

func (t execTask) runCommand(ctx context.Context, root *os.Root, command []string) error {
	dir := path.Join(t.prefix, t.addOnDir)
	var argv []string
	switch command[0] {
	case "@php", "@composer":
		argv = append([]string{strings.TrimPrefix(command[0], "@")}, command[1:]...)
	case "@xf":
		dir = "."
		argv = append([]string{"php", "cmd.php"}, command[1:]...)
	default:
		return runCommand(ctx, root, t.streams, t.addOnDir, command[0], command[1:]...)
	}

	return xfcmd.RunInDir(ctx, t.installationDir, dir, t.streams.stdin, t.streams.stdout, t.streams.stderr, argv...)
}
