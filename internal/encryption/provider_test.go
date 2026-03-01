package encryption

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// --- provider.go tests ---

func TestGenerateKey(t *testing.T) {
	k1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(k1) != KeySize {
		t.Fatalf("key length = %d, want %d", len(k1), KeySize)
	}

	k2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey (second call): %v", err)
	}
	if bytes.Equal(k1, k2) {
		t.Fatal("two generated keys are identical")
	}
}

func TestValidateKey(t *testing.T) {
	tests := []struct {
		name    string
		key     []byte
		wantErr bool
	}{
		{"valid 32 bytes", make([]byte, 32), false},
		{"too short", make([]byte, 16), true},
		{"too long", make([]byte, 64), true},
		{"empty", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestKeyFingerprint(t *testing.T) {
	k1, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	k2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	fp1a := KeyFingerprint(k1)
	fp1b := KeyFingerprint(k1)
	fp2 := KeyFingerprint(k2)

	if fp1a != fp1b {
		t.Errorf("same key produced different fingerprints: %q vs %q", fp1a, fp1b)
	}
	if fp1a == fp2 {
		t.Error("different keys produced the same fingerprint")
	}
	if len(fp1a) == 0 {
		t.Error("fingerprint is empty")
	}
}

// --- keyfile provider tests ---

func TestKeyfileProvider_GetKey(t *testing.T) {
	key, err := GenerateKey(); if err != nil { t.Fatalf("GenerateKey: %v", err) }
	encoded := base64.StdEncoding.EncodeToString(key)

	path := filepath.Join(t.TempDir(), "test.key")
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	p := NewKeyfileProvider(path)
	got, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Error("key mismatch")
	}
	if p.Name() != "keyfile" {
		t.Errorf("Name() = %q, want %q", p.Name(), "keyfile")
	}
}

func TestKeyfileProvider_FileNotFound(t *testing.T) {
	p := NewKeyfileProvider(filepath.Join(t.TempDir(), "nonexistent.key"))
	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestKeyfileProvider_InvalidKey(t *testing.T) {
	shortKey := make([]byte, 16)
	encoded := base64.StdEncoding.EncodeToString(shortKey)

	path := filepath.Join(t.TempDir(), "short.key")
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	p := NewKeyfileProvider(path)
	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestKeyfileProvider_InvalidBase64(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.key")
	if err := os.WriteFile(path, []byte("not-valid-base64!!!"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	p := NewKeyfileProvider(path)
	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

// --- env provider tests ---

func TestEnvProvider_GetKey(t *testing.T) {
	key, err := GenerateKey(); if err != nil { t.Fatalf("GenerateKey: %v", err) }
	encoded := base64.StdEncoding.EncodeToString(key)
	t.Setenv("TEST_ENCRYPTION_KEY", encoded)

	p := NewEnvProvider("TEST_ENCRYPTION_KEY")
	got, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Error("key mismatch")
	}
	if p.Name() != "env" {
		t.Errorf("Name() = %q, want %q", p.Name(), "env")
	}
}

func TestEnvProvider_DefaultEnvVar(t *testing.T) {
	p := NewEnvProvider("")
	if p.envVar != DefaultEnvVar {
		t.Errorf("envVar = %q, want %q", p.envVar, DefaultEnvVar)
	}
}

func TestEnvProvider_Unset(t *testing.T) {
	t.Setenv("TEST_UNSET_KEY", "")
	os.Unsetenv("TEST_UNSET_KEY")

	p := NewEnvProvider("TEST_UNSET_KEY")
	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
}

func TestEnvProvider_InvalidKey(t *testing.T) {
	shortKey := make([]byte, 16)
	t.Setenv("TEST_SHORT_KEY", base64.StdEncoding.EncodeToString(shortKey))

	p := NewEnvProvider("TEST_SHORT_KEY")
	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error for wrong-size key")
	}
}

// --- passphrase provider tests ---

func TestPassphraseProvider_DeriveKey(t *testing.T) {
	salt := make([]byte, 16)
	p := NewPassphraseProvider("my-secret-passphrase", salt)

	k1, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if len(k1) != KeySize {
		t.Fatalf("key length = %d, want %d", len(k1), KeySize)
	}

	k2, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey (second call): %v", err)
	}
	if !bytes.Equal(k1, k2) {
		t.Error("same passphrase+salt produced different keys")
	}
	if p.Name() != "passphrase" {
		t.Errorf("Name() = %q, want %q", p.Name(), "passphrase")
	}
}

func TestPassphraseProvider_DifferentPassphrase(t *testing.T) {
	salt := make([]byte, 16)
	k1, _ := NewPassphraseProvider("passphrase-one", salt).GetKey(context.Background())
	k2, _ := NewPassphraseProvider("passphrase-two", salt).GetKey(context.Background())

	if bytes.Equal(k1, k2) {
		t.Error("different passphrases produced the same key")
	}
}

func TestPassphraseProvider_DifferentSalt(t *testing.T) {
	salt1 := make([]byte, 16)
	salt2 := make([]byte, 16)
	salt2[0] = 1

	k1, _ := NewPassphraseProvider("same-passphrase", salt1).GetKey(context.Background())
	k2, _ := NewPassphraseProvider("same-passphrase", salt2).GetKey(context.Background())

	if bytes.Equal(k1, k2) {
		t.Error("different salts produced the same key")
	}
}

func TestPassphraseProvider_ShortSalt(t *testing.T) {
	p := NewPassphraseProvider("passphrase", make([]byte, 8))
	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error for short salt")
	}
}

// --- exec provider tests ---

func TestExecProvider_GetKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(key)

	p := NewExecProvider("echo " + encoded)
	got, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Error("key mismatch")
	}
	if p.Name() != "exec" {
		t.Errorf("Name() = %q, want %q", p.Name(), "exec")
	}
}

func TestExecProvider_CommandFails(t *testing.T) {
	p := NewExecProvider("false")
	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error for failing command")
	}
}

func TestExecProvider_InvalidOutput(t *testing.T) {
	p := NewExecProvider("echo not-valid-base64!!!")
	_, err := p.GetKey(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid base64 output")
	}
}
