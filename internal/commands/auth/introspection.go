package auth

import (
	"context"
	"time"

	"github.com/xenforo-ltd/cli/internal/auth"
	"github.com/xenforo-ltd/cli/internal/config"
)

// introspectAuthToken checks an access token with the configured server within ten seconds.
func introspectAuthToken(ctx context.Context, token *auth.Token) (*auth.IntrospectResponse, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return auth.NewOAuthClient(&cfg.OAuth).IntrospectToken(ctx, token.AccessToken)
}
