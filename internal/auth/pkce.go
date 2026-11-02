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

// pkceParams contains a random verifier, its S256 challenge, and random state
// for CSRF protection in an OAuth flow.
type pkceParams struct {
	codeVerifier string

	codeChallenge string

	codeChallengeMethod string

	State string
}

// GeneratePKCE generates new PKCE parameters for an OAuth flow.
func GeneratePKCE() (*pkceParams, error) {
	verifierBytes := make([]byte, codeVerifierBytes)
	rand.Read(verifierBytes)

	codeVerifier := base64URLEncode(verifierBytes)

	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64URLEncode(hash[:])

	stateBytes := make([]byte, 32)
	rand.Read(stateBytes)

	state := base64URLEncode(stateBytes)

	return &pkceParams{
		codeVerifier:        codeVerifier,
		codeChallenge:       codeChallenge,
		codeChallengeMethod: "S256",
		State:               state,
	}, nil
}

// base64URLEncode encodes data using unpadded URL-safe base64.
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
