package cmd

import "errors"

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

	// ErrGetCurrentDirectory is returned when the current directory cannot be determined.
	ErrGetCurrentDirectory = errors.New("failed to get current directory")

	// ErrUsernameTooShort is returned when username validation fails.
	ErrUsernameTooShort = errors.New("username must be at least 3 characters")

	// ErrPasswordRequired is returned when password is not provided.
	ErrPasswordRequired = errors.New("password is required")

	// ErrInvalidEmail is returned when email validation fails.
	ErrInvalidEmail = errors.New("invalid email address")

	// ErrAdminUserRequired is returned when admin username is not provided.
	ErrAdminUserRequired = errors.New("admin username is required")

	// ErrValidEmailRequired is returned when admin email is not provided.
	ErrValidEmailRequired = errors.New("valid admin email is required")
)
