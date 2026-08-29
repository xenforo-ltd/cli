package main

import (
	"errors"
	"fmt"
)

// kindError classifies an error for errors.Is without letting the
// classification sentinel's text into the user-visible message.
type kindError struct {
	err  error
	kind error
}

func (e *kindError) Error() string   { return e.err.Error() }
func (e *kindError) Unwrap() []error { return []error{e.err, e.kind} }

func markAs(kind error, format string, a ...any) error {
	return &kindError{err: fmt.Errorf(format, a...), kind: kind}
}

// hintedError carries a remediation line shown under the error message.
type hintedError struct {
	err  error
	hint string
}

func (e *hintedError) Error() string { return e.err.Error() }
func (e *hintedError) Unwrap() error { return e.err }

func withHint(err error, hint string) error {
	return &hintedError{err: err, hint: hint}
}

func hintOf(err error) string {
	var h *hintedError
	if errors.As(err, &h) {
		return h.hint
	}
	return ""
}

// ErrCancelled marks a deliberate user cancellation; it exits 0 silently.
var ErrCancelled = errors.New("cancelled")

// exitCodeError requests a bare process exit with the child's status,
// printing nothing: the child already reported its own failure.
type exitCodeError struct{ code int }

func newExitCodeError(code int) error { return &exitCodeError{code: code} }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

var (
	// ErrInvalidInput indicates invalid user input.
	ErrInvalidInput = errors.New("invalid input")

	// ErrAuthFailed indicates an authentication operation failed.
	ErrAuthFailed = errors.New("authentication failed")

	// ErrKeychainUnavailable indicates the system keychain is not available.
	ErrKeychainUnavailable = errors.New("keychain unavailable")

	// ErrNotFound indicates a requested resource was not found.
	ErrNotFound = errors.New("not found")

	// ErrForbidden indicates access to a resource is forbidden.
	ErrForbidden = errors.New("forbidden")

	// ErrInternal indicates an internal error.
	ErrInternal = errors.New("internal error")

	// minimumUsernameLength is the shortest admin username accepted across
	// interactive prompts.
	minimumUsernameLength = 3

	// ErrUsernameTooShort is returned when username validation fails.
	ErrUsernameTooShort = fmt.Errorf("username must be at least %d characters", minimumUsernameLength)

	// ErrPasswordRequired is returned when password is not provided.
	ErrPasswordRequired = errors.New("password is required")

	// ErrInvalidEmail is returned when email validation fails.
	ErrInvalidEmail = errors.New("invalid email address")

	// ErrAdminUserRequired is returned when admin username is not provided.
	ErrAdminUserRequired = errors.New("admin username is required")
)
