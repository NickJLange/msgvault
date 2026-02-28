package encryption

import (
	"context"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	minSaltLen    = 16
)

// PassphraseProvider derives a 256-bit key from a passphrase using Argon2id.
type PassphraseProvider struct {
	passphrase string
	salt       []byte
}

// NewPassphraseProvider returns a provider that derives a key from the given passphrase and salt.
// The salt must be at least 16 bytes.
func NewPassphraseProvider(passphrase string, salt []byte) *PassphraseProvider {
	return &PassphraseProvider{passphrase: passphrase, salt: salt}
}

// GetKey derives the encryption key using Argon2id.
func (p *PassphraseProvider) GetKey(ctx context.Context) ([]byte, error) {
	if len(p.salt) < minSaltLen {
		return nil, fmt.Errorf("salt too short: got %d bytes, need at least %d", len(p.salt), minSaltLen)
	}
	key := argon2.IDKey([]byte(p.passphrase), p.salt, argon2Time, argon2Memory, argon2Threads, KeySize)
	return key, nil
}

// Name returns the provider name.
func (p *PassphraseProvider) Name() string { return "passphrase" }
