package initialize

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const minimumUsernameLength = 3

var (
	errUsernameTooShort  = fmt.Errorf("username must be at least %d characters", minimumUsernameLength)
	errPasswordRequired  = errors.New("password is required")
	errInvalidEmail      = errors.New("invalid email address")
	errAdminUserRequired = errors.New("admin username is required")
)

func validateAdminUsername(s string) error {
	if utf8.RuneCountInString(s) < minimumUsernameLength {
		return errUsernameTooShort
	}
	return nil
}
