package auth

import (
	"testing"
	"time"
)

func TestToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "expiring within 30 seconds (considered expired)",
			expiresAt: time.Now().Add(20 * time.Second),
			want:      true,
		},
		{
			name:      "expiring in more than 30 seconds",
			expiresAt: time.Now().Add(45 * time.Second),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &Token{ExpiresAt: tt.expiresAt}
			if got := token.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToken_IsExpiringSoon(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		within    time.Duration
		want      bool
	}{
		{
			name:      "expires in 5 minutes, checking 10 minutes",
			expiresAt: time.Now().Add(5 * time.Minute),
			within:    10 * time.Minute,
			want:      true,
		},
		{
			name:      "expires in 15 minutes, checking 10 minutes",
			expiresAt: time.Now().Add(15 * time.Minute),
			within:    10 * time.Minute,
			want:      false,
		},
		{
			name:      "already expired",
			expiresAt: time.Now().Add(-5 * time.Minute),
			within:    10 * time.Minute,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &Token{ExpiresAt: tt.expiresAt}
			if got := token.IsExpiringSoon(tt.within); got != tt.want {
				t.Errorf("IsExpiringSoon(%v) = %v, want %v", tt.within, got, tt.want)
			}
		})
	}
}

func TestToken_TimeUntilExpiry(t *testing.T) {
	// Token expiring in 1 hour
	expiresAt := time.Now().Add(1 * time.Hour)
	token := &Token{ExpiresAt: expiresAt}

	remaining := token.TimeUntilExpiry()

	if remaining < 59*time.Minute || remaining > 61*time.Minute {
		t.Errorf("TimeUntilExpiry() = %v, want ~1 hour", remaining)
	}
}

// Note: Keychain tests are tricky because they interact with the real system keychain.
// For actual keychain testing, you'd typically:
// 1. Use build tags to skip on CI
// 2. Use a mock interface
// 3. Test manually on the target platforms

func TestKeychain_NewKeychain(t *testing.T) {
	kc := NewKeychain()
	if kc == nil {
		t.Error("NewKeychain() returned nil")
	}
}
