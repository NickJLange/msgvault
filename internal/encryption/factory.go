package encryption

import (
	"fmt"

	"github.com/wesm/msgvault/internal/config"
)

// NewProvider creates a KeyProvider from the encryption configuration.
// dbPath is the database path, used by the keyring provider to scope keys.
func NewProvider(cfg config.EncryptionConfig, dbPath string) (KeyProvider, error) {
	switch cfg.Provider {
	case "keyring", "":
		return NewKeyringProvider(dbPath), nil
	case "keyfile":
		if cfg.Keyfile.Path == "" {
			return nil, fmt.Errorf("encryption provider %q requires [encryption.keyfile] path", cfg.Provider)
		}
		return NewKeyfileProvider(cfg.Keyfile.Path), nil
	case "env":
		return NewEnvProvider(""), nil
	case "passphrase":
		return nil, fmt.Errorf("passphrase provider requires interactive setup; use 'msgvault key init --provider passphrase'")
	case "exec":
		if cfg.Exec.Command == "" {
			return nil, fmt.Errorf("encryption provider %q requires [encryption.exec] command", cfg.Provider)
		}
		return NewExecProvider(cfg.Exec.Command), nil
	case "vault":
		return nil, fmt.Errorf("vault provider is not supported; use keyring, keyfile, env, or exec instead")
	default:
		return nil, fmt.Errorf("unknown encryption provider %q", cfg.Provider)
	}
}
