package clierrors

import (
	"errors"
	"fmt"
	"testing"
)

var (
	errTestPermissionDenied  = errors.New("permission denied")
	errTestUnderlyingError   = errors.New("underlying error")
	errTestInvalidToken      = errors.New("invalid token")
	errTestFileNotFound      = errors.New("file not found")
	errTestConnectionRefused = errors.New("connection refused")
	errTestRegularError      = errors.New("regular error")
)

func TestCLIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CLIError
		expected string
	}{
		{
			name: "without cause",
			err: &CLIError{
				Code:    CodeConfigNotFound,
				Message: "config file not found",
			},
			expected: "[E200] config file not found",
		},
		{
			name: "with cause",
			err: &CLIError{
				Code:    CodeConfigReadFailed,
				Message: "failed to read config",
				Cause:   errTestPermissionDenied,
			},
			expected: "[E203] failed to read config: permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCLIError_Unwrap(t *testing.T) {
	cause := errTestUnderlyingError
	err := &CLIError{
		Code:    CodeInternal,
		Message: "something went wrong",
		Cause:   cause,
	}

	if unwrapped := err.Unwrap(); !errors.Is(unwrapped, cause) {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}
}

func TestCLIError_JSON(t *testing.T) {
	err := &CLIError{
		Code:    CodeAuthFailed,
		Message: "authentication failed",
	}

	json := err.JSON()
	expected := `{"code":"E301","message":"authentication failed"}`
	if json != expected {
		t.Errorf("JSON() = %q, want %q", json, expected)
	}

	errWithCause := &CLIError{
		Code:    CodeAuthFailed,
		Message: "authentication failed",
		Cause:   errTestInvalidToken,
	}

	jsonWithCause := errWithCause.JSON()
	expectedWithCause := `{"code":"E301","message":"authentication failed","cause":"invalid token"}`
	if jsonWithCause != expectedWithCause {
		t.Errorf("JSON() = %q, want %q", jsonWithCause, expectedWithCause)
	}
}

func TestNew(t *testing.T) {
	err := New(CodeValidationFailed, "invalid input")

	if err.Code != CodeValidationFailed {
		t.Errorf("Code = %v, want %v", err.Code, CodeValidationFailed)
	}
	if err.Message != "invalid input" {
		t.Errorf("Message = %q, want %q", err.Message, "invalid input")
	}
	if err.Cause != nil {
		t.Errorf("Cause = %v, want nil", err.Cause)
	}
}

func TestWrap(t *testing.T) {
	cause := errTestFileNotFound
	err := Wrap(CodeFileNotFound, "failed to load file", cause)

	if err.Code != CodeFileNotFound {
		t.Errorf("Code = %v, want %v", err.Code, CodeFileNotFound)
	}
	if !errors.Is(err.Cause, cause) {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewf(t *testing.T) {
	err := Newf(CodeInvalidInput, "invalid value: %d", 42)

	if err.Message != "invalid value: 42" {
		t.Errorf("Message = %q, want %q", err.Message, "invalid value: 42")
	}
}

func TestWrapf(t *testing.T) {
	cause := errTestConnectionRefused
	err := Wrapf(CodeAPIRequestFailed, cause, "failed to connect to %s", "api.example.com")

	if err.Message != "failed to connect to api.example.com" {
		t.Errorf("Message = %q, want %q", err.Message, "failed to connect to api.example.com")
	}
	if !errors.Is(err.Cause, cause) {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestIs(t *testing.T) {
	err := New(CodeAuthRequired, "login required")

	if !Is(err, CodeAuthRequired) {
		t.Error("Is() returned false, want true")
	}
	if Is(err, CodeAuthFailed) {
		t.Error("Is() returned true for different code, want false")
	}
	if Is(errTestRegularError, CodeAuthRequired) {
		t.Error("Is() returned true for non-CLIError, want false")
	}
}

func TestIsWrapped(t *testing.T) {
	err := New(CodeAuthRequired, "login required")
	wrapped := fmt.Errorf("wrapped: %w", err)

	if !Is(wrapped, CodeAuthRequired) {
		t.Error("Is() returned false for wrapped CLIError, want true")
	}
}

func TestGetCode(t *testing.T) {
	cliErr := New(CodeDockerNotRunning, "docker is not running")
	if code := GetCode(cliErr); code != CodeDockerNotRunning {
		t.Errorf("GetCode() = %v, want %v", code, CodeDockerNotRunning)
	}

	regularErr := errTestRegularError
	if code := GetCode(regularErr); code != CodeUnknown {
		t.Errorf("GetCode() = %v, want %v", code, CodeUnknown)
	}
}
