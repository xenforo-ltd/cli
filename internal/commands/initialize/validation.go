package initialize

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// minimumUsernameLength is the minimum number of Unicode code points in a username.
const minimumUsernameLength = 3

var (
	// errUsernameTooShort reports a username below the minimum length.
	errUsernameTooShort = fmt.Errorf("username must be at least %d characters", minimumUsernameLength)
	// errPasswordRequired reports an empty or whitespace-only password.
	errPasswordRequired = errors.New("password is required")
	// errInvalidEmail reports an email address without an at sign.
	errInvalidEmail = errors.New("invalid email address")
	// errAdminUserRequired reports a missing administrator username.
	errAdminUserRequired = errors.New("admin username is required")
)

// validateAdminUsername requires a username of at least minimumUsernameLength runes.
func validateAdminUsername(s string) error {
	if utf8.RuneCountInString(s) < minimumUsernameLength {
		return errUsernameTooShort
	}

	return nil
}

// validatePassword rejects empty and whitespace-only passwords.
func validatePassword(s string) error {
	if strings.TrimSpace(s) == "" {
		return errPasswordRequired
	}

	return nil
}

// validateEmail requires an at sign in the trimmed address.
func validateEmail(s string) error {
	if !strings.Contains(strings.TrimSpace(s), "@") {
		return errInvalidEmail
	}

	return nil
}
