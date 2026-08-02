package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

const nodeTokenBytes = 32

// GenerateNodeToken returns a cryptographically random hex token for one agent node.
func GenerateNodeToken() (string, error) {
	buf := make([]byte, nodeTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate node token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// HashNodeToken returns a SHA-256 hex digest of the token (high-entropy tokens).
func HashNodeToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ValidNodeToken compares a plaintext token against a stored hash in constant time.
func ValidNodeToken(provided, storedHash string) bool {
	if storedHash == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(HashNodeToken(provided)), []byte(storedHash)) == 1
}
