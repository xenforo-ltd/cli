package auth

import "errors"

var (
	// ErrAuthRequired indicates that authentication is required.
	ErrAuthRequired = errors.New("authentication required")

	// ErrAuthFailed indicates an authentication operation failed.
	ErrAuthFailed = errors.New("authentication failed")

	// ErrAuthExpired indicates the authentication token has expired.
	ErrAuthExpired = errors.New("authentication expired")

	// ErrInvalidInput indicates invalid input to an auth function.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnsupported indicates an unsupported operation or platform.
	ErrUnsupported = errors.New("unsupported")
)
