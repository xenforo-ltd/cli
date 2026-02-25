// Package clierrors defines structured error types for the CLI.
package clierrors

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Code represents a machine-readable error code.
type Code string

// Error codes for different error categories.
const (
	// CodeUnknown represents general errors (1xx).
	CodeUnknown        Code = "E100"
	CodeInternal       Code = "E101"
	CodeNotImplemented Code = "E102"

	// CodeConfigNotFound represents configuration errors (2xx).
	CodeConfigNotFound    Code = "E200"
	CodeConfigInvalid     Code = "E201"
	CodeConfigWriteFailed Code = "E202"
	CodeConfigReadFailed  Code = "E203"

	// CodeAuthRequired represents authentication errors (3xx).
	CodeAuthRequired        Code = "E300"
	CodeAuthFailed          Code = "E301"
	CodeAuthExpired         Code = "E302"
	CodeAuthRevoked         Code = "E303"
	CodeKeychainUnavailable Code = "E310"
	CodeKeychainReadFailed  Code = "E311"
	CodeKeychainWriteFailed Code = "E312"

	// CodeAPIRequestFailed represents API errors (4xx).
	CodeAPIRequestFailed   Code = "E400"
	CodeAPIResponseInvalid Code = "E401"
	CodeAPIUnauthorized    Code = "E402"
	CodeAPIForbidden       Code = "E403"
	CodeAPINotFound        Code = "E404"
	CodeAPIRateLimited     Code = "E429"

	// CodeFileNotFound represents File/IO errors (5xx).
	CodeFileNotFound     Code = "E500"
	CodeFileReadFailed   Code = "E501"
	CodeFileWriteFailed  Code = "E502"
	CodeDirNotEmpty      Code = "E503"
	CodeDirCreateFailed  Code = "E504"
	CodeDownloadFailed   Code = "E510"
	CodeChecksumMismatch Code = "E511"

	// CodeDockerNotRunning represents Docker errors (6xx).
	CodeDockerNotRunning        Code = "E600"
	CodeDockerCommandFailed     Code = "E601"
	CodeDockerEnvNotInitialized Code = "E602"

	// CodeGitNotFound represents Git/Repo errors (65x).
	CodeGitNotFound      Code = "E650"
	CodeGitCommandFailed Code = "E651"

	// CodeValidationFailed represents Validation errors (7xx).
	CodeValidationFailed Code = "E700"
	CodeInvalidInput     Code = "E701"
	CodeVersionInvalid   Code = "E702"

	// CodeNetworkFailed represents Network errors (8xx).
	CodeNetworkFailed  Code = "E800"
	CodeNetworkTimeout Code = "E801"

	// CodeUpdateFailed represents Update errors (9xx).
	CodeUpdateFailed     Code = "E900"
	CodeUpdateNotAllowed Code = "E901"
)

// CLIError represents a structured error with a code and message.
type CLIError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

// Newf creates a new CLI error with a formatted message.
func Newf(code Code, format string, args ...any) *CLIError {
	return &CLIError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// New creates a new CLI error.
func New(code Code, message string) *CLIError {
	return &CLIError{
		Code:    code,
		Message: message,
	}
}

// Wrap creates a CLI error wrapping another error.
func Wrap(code Code, message string, cause error) *CLIError {
	return &CLIError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// Wrapf creates a CLI error wrapping another error with a formatted message.
func Wrapf(code Code, cause error, format string, args ...any) *CLIError {
	return &CLIError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Cause:   cause,
	}
}

// Is checks if an error has a specific code.
func Is(err error, code Code) bool {
	if cliErr, ok := errors.AsType[*CLIError](err); ok {
		return cliErr.Code == code
	}

	return false
}

// GetCode extracts the code from a CLI error.
func GetCode(err error) Code {
	if cliErr, ok := errors.AsType[*CLIError](err); ok {
		return cliErr.Code
	}

	return CodeUnknown
}

func (e *CLIError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}

	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *CLIError) Unwrap() error {
	return e.Cause
}

// JSON returns the error as a JSON string.
func (e *CLIError) JSON() string {
	type jsonError struct {
		Code    Code   `json:"code"`
		Message string `json:"message"`
		Cause   string `json:"cause,omitempty"`
	}

	je := jsonError{
		Code:    e.Code,
		Message: e.Message,
	}
	if e.Cause != nil {
		je.Cause = e.Cause.Error()
	}

	b, _ := json.Marshal(je)

	return string(b)
}
