package php

import (
	"context"

	"github.com/xenforo-ltd/cli/internal/passthrough"
)

func Run(ctx context.Context, args []string) error { return passthrough.PHP(ctx, args, false) }
