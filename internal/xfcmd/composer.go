package xfcmd

import (
	"context"
	"io"
)

// RunComposer runs Composer in the XenForo environment at xfDir, using exec for
// a running service or run for a stopped service. If no environment is configured,
// it runs Composer on the host in xfDir. Environment failures do not fall back
// to the host. Output writers must be non-nil.
func RunComposer(ctx context.Context, xfDir string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	return RunInDir(ctx, xfDir, ".", stdin, stdout, stderr, append([]string{"composer"}, args...)...)
}
