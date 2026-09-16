package main

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"charm.land/huh/v2"
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
	if h, ok := errors.AsType[*hintedError](err); ok {
		return h.hint
	}
	return ""
}

// ErrCancelled marks a deliberate user cancellation, such as choosing Cancel
// from a menu; it exits 0 silently.
var ErrCancelled = errors.New("cancelled")

// promptError classifies a failed huh prompt. Ctrl-C at a prompt surfaces as
// huh.ErrUserAborted (the form runs in raw mode, so no SIGINT is delivered);
// it is treated like any other interrupt and exits 130. Anything else, such as
// no terminal being available, is a real failure and is reported with context
// rather than being mistaken for a cancellation.
func promptError(err error, format string, a ...any) error {
	what := fmt.Sprintf(format, a...)

	if errors.Is(err, huh.ErrUserAborted) {
		return markAs(context.Canceled, "%s interrupted", what)
	}

	return fmt.Errorf("%s: %w", what, err)
}

// validateAdminUsername enforces the minimum username length shared by the
// init wizard and the review screen. Counted in runes: len would measure
// bytes, so a single multi-byte character would pass a three-character
// minimum.
func validateAdminUsername(s string) error {
	if utf8.RuneCountInString(s) < minimumUsernameLength {
		return ErrUsernameTooShort
	}

	return nil
}

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

	// ErrNotFound indicates a requested resource was not found.
	ErrNotFound = errors.New("not found")

	// ErrForbidden indicates access to a resource is forbidden.
	ErrForbidden = errors.New("forbidden")

	// ErrInternal indicates an internal error.
	ErrInternal = errors.New("internal error")

	// ErrGetCurrentDirectory is returned when the current directory cannot be determined.
	ErrGetCurrentDirectory = errors.New("failed to get current directory")
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

	// ErrValidEmailRequired is returned when admin email is not provided.
	ErrValidEmailRequired = errors.New("valid admin email is required")
)
