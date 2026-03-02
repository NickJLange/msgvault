package encryption

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
)

// DefaultEnvVar is the default environment variable for the encryption key.
const DefaultEnvVar = "MSGVAULT_ENCRYPTION_KEY"

// EnvProvider reads a base64-encoded 256-bit key from an environment variable.
type EnvProvider struct {
	envVar string
}

// NewEnvProvider returns a provider that reads a key from the given environment variable.
// If envVar is empty, DefaultEnvVar is used.
func NewEnvProvider(envVar string) *EnvProvider {
	if envVar == "" {
		envVar = DefaultEnvVar
	}
	return &EnvProvider{envVar: envVar}
}

// GetKey reads and decodes the key from the configured environment variable.
func (p *EnvProvider) GetKey(ctx context.Context) ([]byte, error) {
	raw, ok := os.LookupEnv(p.envVar)
	if !ok {
		return nil, fmt.Errorf("environment variable %q is not set", p.envVar)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding %q: %w", p.envVar, err)
	}
	if err := ValidateKey(key); err != nil {
		return nil, fmt.Errorf("env %q: %w", p.envVar, err)
	}
	return key, nil
}

// Name returns the provider name.
func (p *EnvProvider) Name() string { return "env" }
