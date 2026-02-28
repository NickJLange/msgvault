// Package encryption provides encryption key management for msgvault.
package encryption

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// KeySize is the required key length in bytes (256 bits).
const KeySize = 32

// KeyProvider is the interface for obtaining encryption keys.
type KeyProvider interface {
	GetKey(ctx context.Context) ([]byte, error)
	Name() string
}

// ValidateKey checks that key is exactly KeySize bytes.
func ValidateKey(key []byte) error {
	if len(key) != KeySize {
		return fmt.Errorf("invalid key size: got %d bytes, want %d", len(key), KeySize)
	}
	return nil
}

// GenerateKey generates a random 256-bit encryption key.
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating random key: %w", err)
	}
	return key, nil
}

// KeyFingerprint returns a short fingerprint of the key for display purposes.
func KeyFingerprint(key []byte) string {
	h := sha256.Sum256(key)
	return fmt.Sprintf("SHA-256: %x", h[:8])
}
