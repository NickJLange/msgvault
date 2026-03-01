package encryption

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const keyringService = "msgvault"

// KeyringProvider stores and retrieves encryption keys from the OS keychain
// (macOS Keychain, GNOME Keyring, Windows Credential Manager).
type KeyringProvider struct {
	dbPath string // used as the keyring "user" to support multiple databases
}

// NewKeyringProvider returns a provider that uses the OS keychain.
// dbPath is used to distinguish keys for different databases.
func NewKeyringProvider(dbPath string) *KeyringProvider {
	return &KeyringProvider{dbPath: dbPath}
}

// GetKey retrieves the encryption key from the OS keychain.
func (p *KeyringProvider) GetKey(ctx context.Context) ([]byte, error) {
	encoded, err := keyring.Get(keyringService, p.dbPath)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, fmt.Errorf("%w for %q: %v", ErrKeyNotFound, p.dbPath, err)
		}
		return nil, fmt.Errorf("reading key from OS keyring: %w", err)
	}

	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decoding key from OS keyring: %w", err)
	}
	if err := ValidateKey(key); err != nil {
		return nil, fmt.Errorf("key from OS keyring: %w", err)
	}
	return key, nil
}

// SetKey stores an encryption key in the OS keychain.
func (p *KeyringProvider) SetKey(key []byte) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := keyring.Set(keyringService, p.dbPath, encoded); err != nil {
		return fmt.Errorf("storing key in OS keyring: %w", err)
	}
	return nil
}

// DeleteKey removes the encryption key from the OS keychain.
func (p *KeyringProvider) DeleteKey() error {
	if err := keyring.Delete(keyringService, p.dbPath); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("deleting key from OS keyring: %w", err)
	}
	return nil
}

// Name returns the provider name.
func (p *KeyringProvider) Name() string { return "keyring" }
