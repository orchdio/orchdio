package soundcloud_auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/oauth2"
)

// PKCEParams holds the code verifier and code challenge for PKCE flow
type PKCEParams struct {
	CodeVerifier  string
	CodeChallenge string
}

// GeneratePKCEParams generates a code verifier and code challenge for PKCE flow.
// SoundCloud requires PKCE (Proof Key for Code Exchange) for OAuth 2.1.
// The code challenge method used is S256 (SHA256).
func GeneratePKCEParams() (*PKCEParams, error) {
	// Generate code verifier (43-128 characters)
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}

	// Generate code challenge using S256 method
	challenge := generateCodeChallenge(verifier)

	return &PKCEParams{
		CodeVerifier:  verifier,
		CodeChallenge: challenge,
	}, nil
}

// generateCodeVerifier creates a cryptographically random code verifier
// The verifier is base64url encoded and between 43-128 characters
func generateCodeVerifier() (string, error) {
	// Generate 32 random bytes (will result in 43 characters when base64url encoded)
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}

	// Base64 URL encode without padding
	verifier := base64.RawURLEncoding.EncodeToString(bytes)
	return verifier, nil
}

// generateCodeChallenge creates a code challenge from the verifier using S256 method
func generateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	return challenge
}

// PKCEAuthCodeOptions returns the oauth2.AuthCodeOption slice for authorization URL
// These options include the code_challenge and code_challenge_method parameters
func (p *PKCEParams) PKCEAuthCodeOptions() []oauth2.AuthCodeOption {
	return []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("code_challenge", p.CodeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	}
}

// PKCETokenOptions returns the oauth2.AuthCodeOption slice for token exchange
// This includes the code_verifier parameter
func (p *PKCEParams) PKCETokenOptions() []oauth2.AuthCodeOption {
	return []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("code_verifier", p.CodeVerifier),
	}
}
