package auth

import "time"

// Token holds OAuth credentials, expiry, issue time, scope, and the issuing
// base URL. External tokens have no local expiry or refresh metadata and the
// External marker is not serialized.
type Token struct {
	External     bool      `json:"-"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
	IssuedAt     time.Time `json:"issued_at"`
	BaseURL      string    `json:"base_url"`
}

// IsExpired reports whether the expiry is less than 30 seconds away or past.
func (t *Token) IsExpired() bool {
	return t.IsExpiringSoon(30 * time.Second)
}

// IsExpiringSoon reports whether the expiry is strictly before now plus within.
// It does not special-case external tokens or a zero expiry.
func (t *Token) IsExpiringSoon(within time.Duration) bool {
	return time.Now().Add(within).After(t.ExpiresAt)
}

// TimeUntilExpiry returns the duration until the token expires.
func (t *Token) TimeUntilExpiry() time.Duration {
	return time.Until(t.ExpiresAt)
}
