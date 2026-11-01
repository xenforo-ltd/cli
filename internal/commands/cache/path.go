package cache

import (
	"context"
	"fmt"

	"github.com/xenforo-ltd/cli/internal/config"
)

func Path(_ context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Bare output: consumed by shell substitution, e.g. open $(xf cache path).
	fmt.Println(cfg.CachePath)

	return nil
}
