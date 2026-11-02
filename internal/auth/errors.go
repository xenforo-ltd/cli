package auth

import (
	"errors"
	"strings"
)

var (
	ErrStoreUnavailable = errors.New("credential storage unavailable")
	ErrConfigMismatch   = errors.New("authentication configuration mismatch")

	// ErrAuthRequired indicates that authentication is required.
	ErrAuthRequired = errors.New("authentication required")

	// errAuthFailed indicates an authentication operation failed.
	errAuthFailed = errors.New("authentication failed")

	// errAuthExpired indicates the authentication token has expired.
	errAuthExpired = errors.New("authentication expired")

	// ErrInvalidInput indicates invalid input to an auth function.
	ErrInvalidInput = errors.New("invalid input")

	// errUnsupported indicates an unsupported operation or platform.
	errUnsupported = errors.New("unsupported")
)

// ErrorMessage removes classification sentinels from user-facing diagnostics.
func ErrorMessage(err error) string {
	message := err.Error()
	for _, kind := range []error{ErrAuthRequired, ErrStoreUnavailable, ErrConfigMismatch, ErrInvalidInput, errUnsupported, errAuthFailed, errAuthExpired} {
		if errors.Is(err, kind) {
			return strings.TrimSuffix(message, ": "+kind.Error())
		}
	}

	return message
}
