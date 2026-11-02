package cache

import (
	"context"
	"fmt"

	"github.com/xenforo-ltd/cli/internal/config"
)

// Path prints the cache directory without decoration for shell substitution.
func Path(_ context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Println(cfg.CachePath)

	return nil
}
