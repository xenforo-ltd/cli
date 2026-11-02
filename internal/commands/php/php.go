package php

import (
	"context"

	"github.com/xenforo-ltd/cli/internal/passthrough"
)

// Run forwards arguments to PHP without enabling Xdebug.
func Run(ctx context.Context, args []string) error { return passthrough.PHP(ctx, args, false) }
