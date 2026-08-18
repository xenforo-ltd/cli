package main

import (
	"encoding/json"
	"testing"
)

// The keys below are consumed by scripts, so they are part of the CLI's
// contract: renaming or dropping one is a breaking change, not a cosmetic
// tidy-up.
func TestAuthStatusJSONKeepsItsEstablishedKeys(t *testing.T) {
	serverValid := true

	data, err := json.Marshal(authStatusJSON{
		Authenticated: true,
		Expired:       false,
		Scope:         "licenses:read",
		IssuedAt:      "2026-04-30T16:11:50Z",
		ExpiresAt:     "2026-04-30T18:11:50Z",
		ServerValid:   &serverValid,
		Username:      "chris",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	want := map[string]any{
		"authenticated": true,
		"expired":       false,
		"scope":         "licenses:read",
		"issued_at":     "2026-04-30T16:11:50Z",
		"expires_at":    "2026-04-30T18:11:50Z",
		"server_valid":  true,
		"username":      "chris",
	}

	for key, wantValue := range want {
		got, ok := decoded[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}

		if got != wantValue {
			t.Errorf("%q = %v, want %v", key, got, wantValue)
		}
	}
}

func TestAuthStatusJSONOmitsUnknownTokenDetails(t *testing.T) {
	data, err := json.Marshal(authStatusJSON{Error: "keychain unavailable"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// authenticated and expired are always present so scripts can branch on
	// them without checking for existence first.
	for _, key := range []string{"authenticated", "expired"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("key %q must always be present", key)
		}
	}

	// Token details are meaningless without a token, so they are omitted
	// rather than reported as empty strings.
	for _, key := range []string{"scope", "issued_at", "expires_at", "server_valid", "username"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("key %q should be omitted when no token is present", key)
		}
	}

	if decoded["error"] != "keychain unavailable" {
		t.Errorf("error = %v, want %q", decoded["error"], "keychain unavailable")
	}
}
