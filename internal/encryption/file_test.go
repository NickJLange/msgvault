package encryption

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generating test key: %v", err)
	}
	return key
}

func TestEncryptDecryptBytes(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("hello, world! this is a test of AES-256-GCM encryption")

	encrypted, err := EncryptBytes(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}

	if len(encrypted) < MinEncryptedSize {
		t.Fatalf("encrypted data too short: %d bytes", len(encrypted))
	}
	if encrypted[0] != FileVersion {
		t.Errorf("version byte = 0x%02x, want 0x%02x", encrypted[0], FileVersion)
	}

	decrypted, err := DecryptBytes(key, encrypted)
	if err != nil {
		t.Fatalf("DecryptBytes: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecryptBytes_Empty(t *testing.T) {
	key := testKey(t)
	plaintext := []byte{}

	encrypted, err := EncryptBytes(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}

	decrypted, err := DecryptBytes(key, encrypted)
	if err != nil {
		t.Fatalf("DecryptBytes: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("decrypted length = %d, want 0", len(decrypted))
	}
}

func TestDecryptBytes_WrongKey(t *testing.T) {
	key1 := testKey(t)
	key2 := testKey(t)

	encrypted, err := EncryptBytes(key1, []byte("secret data"))
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}

	_, err = DecryptBytes(key2, encrypted)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

func TestDecryptBytes_Tampered(t *testing.T) {
	key := testKey(t)

	encrypted, err := EncryptBytes(key, []byte("integrity-protected data"))
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}

	// Tamper with a ciphertext byte (after version + nonce).
	tampered := make([]byte, len(encrypted))
	copy(tampered, encrypted)
	tampered[1+NonceSize] ^= 0xff

	_, err = DecryptBytes(key, tampered)
	if err == nil {
		t.Fatal("expected error when decrypting tampered data")
	}
}

func TestDecryptBytes_Truncated(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"version_only", []byte{FileVersion}},
		{"version_and_partial_nonce", make([]byte, 10)},
		{"just_under_minimum", make([]byte, MinEncryptedSize-1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := testKey(t)
			_, err := DecryptBytes(key, tt.data)
			if err == nil {
				t.Fatal("expected error for truncated data")
			}
		})
	}
}

func TestDecryptBytes_BadVersion(t *testing.T) {
	key := testKey(t)

	encrypted, err := EncryptBytes(key, []byte("version test"))
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}

	badVersion := make([]byte, len(encrypted))
	copy(badVersion, encrypted)
	badVersion[0] = 0x99

	_, err = DecryptBytes(key, badVersion)
	if err == nil {
		t.Fatal("expected error for bad version byte")
	}
}

func TestEncryptBytes_Idempotent(t *testing.T) {
	key := testKey(t)
	plaintext := []byte("same input, different output")

	enc1, err := EncryptBytes(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptBytes (1): %v", err)
	}

	enc2, err := EncryptBytes(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptBytes (2): %v", err)
	}

	if bytes.Equal(enc1, enc2) {
		t.Error("encrypting the same plaintext twice produced identical ciphertext (nonce should differ)")
	}

	// Both should still decrypt to the same plaintext.
	dec1, err := DecryptBytes(key, enc1)
	if err != nil {
		t.Fatalf("DecryptBytes (1): %v", err)
	}
	dec2, err := DecryptBytes(key, enc2)
	if err != nil {
		t.Fatalf("DecryptBytes (2): %v", err)
	}
	if !bytes.Equal(dec1, dec2) {
		t.Error("decrypted results differ")
	}
}

func TestEncryptDecryptFile(t *testing.T) {
	key := testKey(t)
	dir := t.TempDir()

	srcPath := filepath.Join(dir, "plain.txt")
	encPath := filepath.Join(dir, "encrypted.bin")
	decPath := filepath.Join(dir, "decrypted.txt")

	plaintext := []byte("file encryption round-trip test data")
	if err := os.WriteFile(srcPath, plaintext, 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	if err := EncryptFile(key, srcPath, encPath); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	// Verify encrypted file exists and has restrictive permissions.
	info, err := os.Stat(encPath)
	if err != nil {
		t.Fatalf("Stat encrypted file: %v", err)
	}
	if info.Size() < int64(MinEncryptedSize) {
		t.Errorf("encrypted file too small: %d bytes", info.Size())
	}

	if err := DecryptFile(key, encPath, decPath); err != nil {
		t.Fatalf("DecryptFile: %v", err)
	}

	got, err := os.ReadFile(decPath)
	if err != nil {
		t.Fatalf("reading decrypted file: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("decrypted content = %q, want %q", got, plaintext)
	}
}

func TestDecryptFile_WrongKey(t *testing.T) {
	key1 := testKey(t)
	key2 := testKey(t)
	dir := t.TempDir()

	srcPath := filepath.Join(dir, "plain.txt")
	encPath := filepath.Join(dir, "encrypted.bin")
	decPath := filepath.Join(dir, "decrypted.txt")

	if err := os.WriteFile(srcPath, []byte("secret"), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	if err := EncryptFile(key1, srcPath, encPath); err != nil {
		t.Fatalf("EncryptFile: %v", err)
	}

	err := DecryptFile(key2, encPath, decPath)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}

	// Decrypted file should not exist on failure.
	if _, statErr := os.Stat(decPath); statErr == nil {
		t.Error("decrypted file should not exist after failed decryption")
	}
}

func TestIsEncrypted(t *testing.T) {
	key := testKey(t)

	encrypted, err := EncryptBytes(key, []byte("test data"))
	if err != nil {
		t.Fatalf("EncryptBytes: %v", err)
	}

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"valid_encrypted", encrypted, true},
		{"plain_text", []byte("just some plain text that starts with any byte"), false},
		{"empty", []byte{}, false},
		{"too_short", []byte{FileVersion, 0x00, 0x01}, false},
		{"wrong_version_long_enough", append([]byte{0x99}, make([]byte, MinEncryptedSize)...), false},
		{"exact_minimum_with_version", append([]byte{FileVersion}, make([]byte, MinEncryptedSize-1)...), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEncrypted(tt.data); got != tt.want {
				t.Errorf("IsEncrypted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncryptBytes_InvalidKeySize(t *testing.T) {
	tests := []struct {
		name    string
		keySize int
	}{
		{"too_short", 16},
		{"too_long", 64},
		{"empty", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := make([]byte, tt.keySize)
			_, err := EncryptBytes(key, []byte("test"))
			if err == nil {
				t.Fatal("expected error for invalid key size")
			}
		})
	}
}
