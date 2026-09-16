package main

import (
	"encoding/json"
	"testing"
)

// The keys below are consumed by scripts, so they are part of the CLI's
// contract: renaming or dropping one is a breaking change, not a cosmetic
// tidy-up.
func TestAuthStatusJSONKeepsItsEstablishedKeys(t *testing.T) {
	data, err := json.Marshal(authStatusJSON{
		Authenticated: new(true),
		Expired:       new(false),
		Scope:         "licenses:read",
		IssuedAt:      "2026-04-30T16:11:50Z",
		ExpiresAt:     "2026-04-30T18:11:50Z",
		ServerValid:   new(true),
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
	data, err := json.Marshal(authStatusJSON{
		Authenticated: new(false),
		Reason:        "store_unavailable",
		Error:         "keychain unavailable",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// authenticated and expired are always present so scripts can branch on
	// them without checking for existence first. With no token loaded there is
	// nothing to judge expiry against, so expired is present but null.
	for _, key := range []string{"authenticated", "expired"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("key %q must always be present", key)
		}
	}

	if decoded["expired"] != nil {
		t.Errorf("expired = %v, want null when no token is present", decoded["expired"])
	}

	// The failure reason survives the shared shape; token details stay out.
	if decoded["reason"] != "store_unavailable" {
		t.Errorf("reason = %v, want %q", decoded["reason"], "store_unavailable")
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

// The XF_TOKEN response keeps the keys scripts already branch on. They are
// present even when introspection failed, as explicit nulls, rather than being
// dropped the way the stored-token shape drops meaningless details.
func TestEnvironmentAuthStatusJSONKeepsNullableKeysWhenUnknown(t *testing.T) {
	data, err := json.Marshal(environmentAuthStatusJSON{Source: "environment"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"authenticated", "server_valid", "expires_at", "issued_at", "expired"} {
		got, ok := decoded[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}

		if got != nil {
			t.Errorf("%q = %v, want null when unknown", key, got)
		}
	}

	if decoded["source"] != "environment" {
		t.Errorf("source = %v, want %q", decoded["source"], "environment")
	}

	// These only exist once introspection has something to say.
	for _, key := range []string{"scope", "username", "error"} {
		if _, ok := decoded[key]; ok {
			t.Errorf("key %q should be omitted when introspection has not succeeded", key)
		}
	}
}

func TestEnvironmentAuthStatusJSONSerializesValidatedClaims(t *testing.T) {
	const (
		scope     = "licenses:read"
		username  = "chris"
		expiresAt = "2026-04-30T18:11:50Z"
		issuedAt  = "2026-04-30T16:11:50Z"
	)

	data, err := json.Marshal(environmentAuthStatusJSON{
		Authenticated: new(true),
		Source:        "environment",
		ServerValid:   new(true),
		ExpiresAt:     new(expiresAt),
		IssuedAt:      new(issuedAt),
		Expired:       new(false),
		Scope:         new(scope),
		Username:      new(username),
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
		"source":        "environment",
		"server_valid":  true,
		"expires_at":    expiresAt,
		"issued_at":     issuedAt,
		"expired":       false,
		"scope":         scope,
		"username":      username,
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

	if _, ok := decoded["error"]; ok {
		t.Errorf("error should be omitted on the success path: %v", decoded["error"])
	}
}

// Introspection can succeed while returning empty claims. The original
// map-based output included scope and username as empty strings in that case,
// so the typed shape must not drop them.
func TestEnvironmentAuthStatusJSONKeepsEmptyValidatedScopeAndUsername(t *testing.T) {
	data, err := json.Marshal(environmentAuthStatusJSON{
		Authenticated: new(true),
		Source:        "environment",
		ServerValid:   new(true),
		Expired:       new(false),
		Scope:         new(""),
		Username:      new(""),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"scope", "username"} {
		got, ok := decoded[key]
		if !ok {
			t.Errorf("missing key %q", key)
			continue
		}

		if got != "" {
			t.Errorf("%q = %v, want empty string", key, got)
		}
	}
}
