package encryption

import (
	"testing"

	"github.com/wesm/msgvault/internal/config"
)

func TestNewProvider_Keyring(t *testing.T) {
	cfg := config.EncryptionConfig{Provider: "keyring"}
	p, err := NewProvider(cfg, "/tmp/test.db")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "keyring" {
		t.Errorf("Name() = %q, want %q", p.Name(), "keyring")
	}
}

func TestNewProvider_DefaultIsKeyring(t *testing.T) {
	cfg := config.EncryptionConfig{Provider: ""}
	p, err := NewProvider(cfg, "/tmp/test.db")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "keyring" {
		t.Errorf("Name() = %q, want %q", p.Name(), "keyring")
	}
}

func TestNewProvider_Keyfile(t *testing.T) {
	cfg := config.EncryptionConfig{
		Provider: "keyfile",
		Keyfile:  config.KeyfileConfig{Path: "/tmp/key.txt"},
	}
	p, err := NewProvider(cfg, "/tmp/test.db")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "keyfile" {
		t.Errorf("Name() = %q, want %q", p.Name(), "keyfile")
	}
}

func TestNewProvider_KeyfileMissingPath(t *testing.T) {
	cfg := config.EncryptionConfig{Provider: "keyfile"}
	_, err := NewProvider(cfg, "/tmp/test.db")
	if err == nil {
		t.Fatal("NewProvider should fail when keyfile path is empty")
	}
}

func TestNewProvider_Env(t *testing.T) {
	cfg := config.EncryptionConfig{Provider: "env"}
	p, err := NewProvider(cfg, "/tmp/test.db")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "env" {
		t.Errorf("Name() = %q, want %q", p.Name(), "env")
	}
}

func TestNewProvider_Exec(t *testing.T) {
	cfg := config.EncryptionConfig{
		Provider: "exec",
		Exec:     config.ExecConfig{Command: "echo test"},
	}
	p, err := NewProvider(cfg, "/tmp/test.db")
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.Name() != "exec" {
		t.Errorf("Name() = %q, want %q", p.Name(), "exec")
	}
}

func TestNewProvider_ExecMissingCommand(t *testing.T) {
	cfg := config.EncryptionConfig{Provider: "exec"}
	_, err := NewProvider(cfg, "/tmp/test.db")
	if err == nil {
		t.Fatal("NewProvider should fail when exec command is empty")
	}
}

func TestNewProvider_Unknown(t *testing.T) {
	cfg := config.EncryptionConfig{Provider: "magic"}
	_, err := NewProvider(cfg, "/tmp/test.db")
	if err == nil {
		t.Fatal("NewProvider should fail for unknown provider")
	}
}

func TestNewProvider_Passphrase(t *testing.T) {
	cfg := config.EncryptionConfig{Provider: "passphrase"}
	_, err := NewProvider(cfg, "/tmp/test.db")
	if err == nil {
		t.Fatal("NewProvider should fail for passphrase (requires interactive setup)")
	}
}

func TestNewProvider_Vault(t *testing.T) {
	cfg := config.EncryptionConfig{Provider: "vault"}
	_, err := NewProvider(cfg, "/tmp/test.db")
	if err == nil {
		t.Fatal("NewProvider should fail for vault (not supported)")
	}
}
