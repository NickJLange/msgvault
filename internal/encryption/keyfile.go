package encryption

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// KeyfileProvider reads a base64-encoded 256-bit key from a file.
type KeyfileProvider struct {
	path string
}

// NewKeyfileProvider returns a provider that reads a key from the given file path.
func NewKeyfileProvider(path string) *KeyfileProvider {
	return &KeyfileProvider{path: path}
}

// GetKey reads and decodes the key from the configured file.
func (p *KeyfileProvider) GetKey(ctx context.Context) ([]byte, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return nil, fmt.Errorf("reading key file %q: %w", p.path, err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("decoding key file %q: %w", p.path, err)
	}
	if err := ValidateKey(key); err != nil {
		return nil, fmt.Errorf("key file %q: %w", p.path, err)
	}
	return key, nil
}

// Name returns the provider name.
func (p *KeyfileProvider) Name() string { return "keyfile" }
