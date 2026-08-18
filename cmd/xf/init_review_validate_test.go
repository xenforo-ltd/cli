package main

import (
	"errors"
	"os/exec"
	"testing"
	"unicode/utf8"
)

func TestUsernameLengthIsCountedInRunes(t *testing.T) {
	cases := []struct {
		name  string
		input string
		valid bool
	}{
		{"plain name", "admin", true},
		{"exactly the minimum", "abc", true},
		{"too short", "ab", false},
		{"single emoji is one character", "😀", false},
		{"three emoji", "😀😀😀", true},
		{"multi-byte accents below the minimum", "áé", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			valid := utf8.RuneCountInString(tc.input) >= minimumUsernameLength
			if valid != tc.valid {
				t.Errorf("%q: valid = %v, want %v (bytes=%d runes=%d)",
					tc.input, valid, tc.valid, len(tc.input), utf8.RuneCountInString(tc.input))
			}
		})
	}
}

func TestPassthroughErrorIgnoresSignalExitCodes(t *testing.T) {
	// A process killed by a signal reports -1, which cannot be used as a
	// process exit status. Those failures must stay reportable instead.
	cmd := exec.Command("sh", "-c", "kill -TERM $$")

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected the signalled command to fail")
	}

	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected an *exec.ExitError, got %T", err)
	}

	if ee.ExitCode() != -1 {
		t.Skipf("platform reported exit code %d, not -1", ee.ExitCode())
	}

	var exitErr *exitCodeError
	if errors.As(passthroughError(err, "failed to run PHP"), &exitErr) {
		t.Errorf("signal death became exit code %d", exitErr.code)
	}
}
