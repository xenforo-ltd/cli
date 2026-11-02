package phpdebug

import (
	"context"

	"github.com/xenforo-ltd/cli/internal/passthrough"
)

// Run forwards arguments to PHP with Xdebug enabled.
func Run(ctx context.Context, args []string) error { return passthrough.PHP(ctx, args, true) }
