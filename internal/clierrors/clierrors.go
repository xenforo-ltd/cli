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
	// General errors (1xx).
	CodeUnknown        Code = "E100"
	CodeInternal       Code = "E101"
	CodeNotImplemented Code = "E102"

	// Configuration errors (2xx).
	CodeConfigNotFound    Code = "E200"
	CodeConfigInvalid     Code = "E201"
	CodeConfigWriteFailed Code = "E202"
	CodeConfigReadFailed  Code = "E203"

	// Authentication errors (3xx).
	CodeAuthRequired        Code = "E300"
	CodeAuthFailed          Code = "E301"
	CodeAuthExpired         Code = "E302"
	CodeAuthRevoked         Code = "E303"
	CodeKeychainUnavailable Code = "E310"
	CodeKeychainReadFailed  Code = "E311"
	CodeKeychainWriteFailed Code = "E312"

	// API errors (4xx).
	CodeAPIRequestFailed   Code = "E400"
	CodeAPIResponseInvalid Code = "E401"
	CodeAPIUnauthorized    Code = "E402"
	CodeAPIForbidden       Code = "E403"
	CodeAPINotFound        Code = "E404"
	CodeAPIRateLimited     Code = "E429"

	// File/IO errors (5xx).
	CodeFileNotFound     Code = "E500"
	CodeFileReadFailed   Code = "E501"
	CodeFileWriteFailed  Code = "E502"
	CodeDirNotEmpty      Code = "E503"
	CodeDirCreateFailed  Code = "E504"
	CodeDownloadFailed   Code = "E510"
	CodeChecksumMismatch Code = "E511"

	// Docker errors (6xx).
	CodeDockerNotRunning        Code = "E600"
	CodeDockerCommandFailed     Code = "E601"
	CodeDockerEnvNotInitialized Code = "E602"

	// Git/Repo errors (65x).
	CodeGitNotFound      Code = "E650"
	CodeGitCommandFailed Code = "E651"

	// Validation errors (7xx).
	CodeValidationFailed Code = "E700"
	CodeInvalidInput     Code = "E701"
	CodeVersionInvalid   Code = "E702"

	// Network errors (8xx).
	CodeNetworkFailed  Code = "E800"
	CodeNetworkTimeout Code = "E801"

	// Update errors (9xx).
	CodeUpdateFailed     Code = "E900"
	CodeUpdateNotAllowed Code = "E901"
)

// CLIError represents a structured error with a code and message.
type CLIError struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
}

func Newf(code Code, format string, args ...any) *CLIError {
	return &CLIError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

func New(code Code, message string) *CLIError {
	return &CLIError{
		Code:    code,
		Message: message,
	}
}

func Wrap(code Code, message string, cause error) *CLIError {
	return &CLIError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

func Wrapf(code Code, cause error, format string, args ...any) *CLIError {
	return &CLIError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Cause:   cause,
	}
}

func Is(err error, code Code) bool {
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr.Code == code
	}
	return false
}

func GetCode(err error) Code {
	if cliErr, ok := err.(*CLIError); ok {
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
