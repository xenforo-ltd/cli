package auth

import "time"

// Token represents an OAuth token with expiry information.
type Token struct {
	// External tokens have no local expiry or refresh metadata.
	External     bool      `json:"-"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
	IssuedAt     time.Time `json:"issued_at"`
	BaseURL      string    `json:"base_url"`
}

// IsExpired checks if the token has expired, accounting for clock skew.
func (t *Token) IsExpired() bool {
	return t.IsExpiringSoon(30 * time.Second)
}

// IsExpiringSoon checks if the token will expire within the given duration.
func (t *Token) IsExpiringSoon(within time.Duration) bool {
	return time.Now().Add(within).After(t.ExpiresAt)
}

// TimeUntilExpiry returns the duration until the token expires.
func (t *Token) TimeUntilExpiry() time.Duration {
	return time.Until(t.ExpiresAt)
}
