package clierror

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"charm.land/huh/v2"
)

// ErrCancelled marks a deliberate user cancellation, such as choosing Cancel
// from a menu; it exits 0 silently.
var ErrCancelled = errors.New("cancelled")

var (
	// ErrInvalidInput indicates invalid user input.
	ErrInvalidInput = errors.New("invalid input")

	// ErrAuthFailed indicates an authentication operation failed.
	ErrAuthFailed = errors.New("authentication failed")

	// ErrNotFound indicates a requested resource was not found.
	ErrNotFound = errors.New("not found")

	// ErrForbidden indicates access to a resource is forbidden.
	ErrForbidden = errors.New("forbidden")
)

// kindError classifies an error for errors.Is without letting the
// classification sentinel's text into the user-visible message.
type kindError struct {
	err  error
	kind error
}

func (e *kindError) Error() string   { return e.err.Error() }
func (e *kindError) Unwrap() []error { return []error{e.err, e.kind} }

func MarkAs(kind error, format string, a ...any) error {
	return &kindError{err: fmt.Errorf(format, a...), kind: kind}
}

// hintedError carries a remediation line shown under the error message.
type hintedError struct {
	err  error
	hint string
}

func (e *hintedError) Error() string { return e.err.Error() }
func (e *hintedError) Unwrap() error { return e.err }

func WithHint(err error, hint string) error {
	return &hintedError{err: err, hint: hint}
}

func HintOf(err error) string {
	if h, ok := errors.AsType[*hintedError](err); ok {
		return h.hint
	}

	return ""
}

// usageError marks an error as caused by incorrect invocation (bad arguments or
// flags), for which printing usage is genuinely helpful.
type usageError struct {
	err error
}

func (e *usageError) Error() string { return e.err.Error() }

func (e *usageError) Unwrap() error { return e.err }

func Usage(err error) error {
	return &usageError{err: err}
}

func IsUsage(err error) bool { _, ok := errors.AsType[*usageError](err); return ok }

// exitCodeError requests a bare process exit with the child's status,
// printing nothing: the child already reported its own failure.
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func WithExitCode(code int) error { return &exitCodeError{code: code} }

func ExitCode(err error) (int, bool) {
	if e, ok := errors.AsType[*exitCodeError](err); ok {
		return e.code, true
	}

	return 0, false
}

// Passthrough converts a child process's non-zero exit into a bare
// exit-code error (the child already printed its own failure); other errors
// are wrapped with context.
func Passthrough(err error, context string) error {
	if err == nil {
		return nil
	}

	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		// A process killed by a signal reports -1, which is not a status any
		// caller can exit with. Those failures are reported normally instead.
		if code := ee.ExitCode(); code >= 0 {
			return WithExitCode(code)
		}
	}

	return fmt.Errorf("%s: %w", context, err)
}

// Prompt classifies a failed huh prompt. Ctrl-C at a prompt surfaces as
// huh.ErrUserAborted (the form runs in raw mode, so no SIGINT is delivered);
// it is treated like any other interrupt and exits 130. Anything else, such as
// no terminal being available, is a real failure and is reported with context
// rather than being mistaken for a cancellation.
func Prompt(err error, format string, a ...any) error {
	what := fmt.Sprintf(format, a...)

	if errors.Is(err, huh.ErrUserAborted) {
		return MarkAs(context.Canceled, "%s interrupted", what)
	}

	return fmt.Errorf("%s: %w", what, err)
}
