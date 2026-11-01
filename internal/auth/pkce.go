package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const (
	// codeVerifierBytes of entropy encode to an 86-character verifier (43-128 allowed).
	codeVerifierBytes = 64
)

// PKCEParams contains the PKCE parameters for an OAuth flow.
type PKCEParams struct {
	// CodeVerifier is the random string used to generate the challenge.
	CodeVerifier string

	// CodeChallenge is the S256 hash of the verifier.
	CodeChallenge string

	// CodeChallengeMethod is always "S256".
	CodeChallengeMethod string

	// State is a random string to prevent CSRF attacks.
	State string
}

// GeneratePKCE generates new PKCE parameters for an OAuth flow.
func GeneratePKCE() (*PKCEParams, error) {
	verifierBytes := make([]byte, codeVerifierBytes)
	rand.Read(verifierBytes)

	codeVerifier := base64URLEncode(verifierBytes)

	// Generate code challenge (S256 = base64url(sha256(verifier)))
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64URLEncode(hash[:])

	stateBytes := make([]byte, 32)
	rand.Read(stateBytes)

	state := base64URLEncode(stateBytes)

	return &PKCEParams{
		CodeVerifier:        codeVerifier,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: "S256",
		State:               state,
	}, nil
}

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
