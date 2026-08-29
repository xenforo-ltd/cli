package main

import (
	"errors"
	"fmt"
	"testing"
)

func TestMarkAsHidesSentinelText(t *testing.T) {
	err := markAs(ErrInvalidInput, "missing required flags: %s", "--license")
	if got := err.Error(); got != "missing required flags: --license" {
		t.Errorf("sentinel text leaked: %q", got)
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Error("errors.Is failed")
	}
}

func TestWithHint(t *testing.T) {
	err := withHint(errors.New("boom"), "Run xf doctor")
	if hintOf(err) != "Run xf doctor" {
		t.Errorf("hint lost")
	}
	wrapped := fmt.Errorf("outer: %w", err)
	if hintOf(wrapped) != "Run xf doctor" {
		t.Errorf("hint not found through wrapping")
	}
	if hintOf(errors.New("plain")) != "" {
		t.Errorf("phantom hint")
	}
}
