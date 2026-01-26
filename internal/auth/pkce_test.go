package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error = %v", err)
	}

	if len(pkce.CodeVerifier) < 43 || len(pkce.CodeVerifier) > 128 {
		t.Errorf("CodeVerifier length = %d, want between 43 and 128", len(pkce.CodeVerifier))
	}

	hash := sha256.Sum256([]byte(pkce.CodeVerifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
	if pkce.CodeChallenge != expectedChallenge {
		t.Errorf("CodeChallenge = %q, want %q", pkce.CodeChallenge, expectedChallenge)
	}

	if pkce.CodeChallengeMethod != "S256" {
		t.Errorf("CodeChallengeMethod = %q, want %q", pkce.CodeChallengeMethod, "S256")
	}

	if len(pkce.State) == 0 {
		t.Error("State is empty")
	}
}

func TestGeneratePKCE_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		pkce, err := GeneratePKCE()
		if err != nil {
			t.Fatalf("GeneratePKCE() error = %v", err)
		}

		if seen[pkce.CodeVerifier] {
			t.Errorf("CodeVerifier collision detected")
		}
		seen[pkce.CodeVerifier] = true

		if seen[pkce.State] {
			t.Errorf("State collision detected")
		}
		seen[pkce.State] = true
	}
}

func TestBase64URLEncode(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "empty",
			input: []byte{},
			want:  "",
		},
		{
			name:  "simple",
			input: []byte("hello"),
			want:  "aGVsbG8",
		},
		{
			name:  "with special chars needing URL encoding",
			input: []byte{0xff, 0xfe, 0xfd},
			want:  "__79",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := base64URLEncode(tt.input)
			if got != tt.want {
				t.Errorf("base64URLEncode() = %q, want %q", got, tt.want)
			}

			if len(got) > 0 && got[len(got)-1] == '=' {
				t.Error("base64URLEncode() should not have padding")
			}
		})
	}
}
