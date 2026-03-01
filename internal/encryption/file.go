// Package encryption provides AES-256-GCM file encryption.
//
// Encrypted file format:
//
//	[version: 1 byte][nonce: 12 bytes][ciphertext+tag: variable]
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wesm/msgvault/internal/fileutil"
)

const (
	// FileVersion is the current encryption format version.
	FileVersion = 0x01
	// NonceSize is the GCM nonce length in bytes.
	NonceSize = 12
	// MinEncryptedSize is version(1) + nonce(12) + GCM tag(16).
	MinEncryptedSize = 1 + NonceSize + 16
)

// EncryptBytes encrypts plaintext using AES-256-GCM with a random nonce.
// The returned data has format: [version][nonce][ciphertext+tag].
func EncryptBytes(key, plaintext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption: key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("encryption: creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encryption: creating GCM: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("encryption: generating nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// [version][nonce][ciphertext+tag]
	out := make([]byte, 0, 1+NonceSize+len(ciphertext))
	out = append(out, FileVersion)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// DecryptBytes decrypts data produced by EncryptBytes.
func DecryptBytes(key, data []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption: key must be 32 bytes, got %d", len(key))
	}
	if len(data) < MinEncryptedSize {
		return nil, fmt.Errorf("encryption: data too short (%d bytes, minimum %d)", len(data), MinEncryptedSize)
	}
	if data[0] != FileVersion {
		return nil, fmt.Errorf("encryption: unsupported version 0x%02x", data[0])
	}

	nonce := data[1 : 1+NonceSize]
	ciphertext := data[1+NonceSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("encryption: creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encryption: creating GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("encryption: decryption failed (wrong key or tampered data): %w", err)
	}
	return plaintext, nil
}

// EncryptFile reads srcPath, encrypts its contents, and writes atomically to dstPath.
func EncryptFile(key []byte, srcPath, dstPath string) error {
	plaintext, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("encryption: reading source file: %w", err)
	}

	encrypted, err := EncryptBytes(key, plaintext)
	if err != nil {
		return err
	}

	// Write to temp file in the same directory, then rename for atomicity.
	dir := filepath.Dir(dstPath)
	tmp, err := os.CreateTemp(dir, ".enc-*")
	if err != nil {
		return fmt.Errorf("encryption: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(encrypted); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("encryption: writing temp file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("encryption: setting permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("encryption: closing temp file: %w", err)
	}

	if err := fileutil.AtomicRename(tmpPath, dstPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("encryption: renaming temp file: %w", err)
	}
	return nil
}

// DecryptFile reads an encrypted file at srcPath and writes the decrypted content
// atomically to dstPath using a temp file and rename.
func DecryptFile(key []byte, srcPath, dstPath string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("encryption: reading encrypted file: %w", err)
	}

	plaintext, err := DecryptBytes(key, data)
	if err != nil {
		return err
	}

	// Write to temp file in the same directory, then rename for atomicity.
	dir := filepath.Dir(dstPath)
	tmp, err := os.CreateTemp(dir, ".dec-*")
	if err != nil {
		return fmt.Errorf("encryption: creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(plaintext); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("encryption: writing temp file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("encryption: setting permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("encryption: closing temp file: %w", err)
	}

	if err := fileutil.AtomicRename(tmpPath, dstPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("encryption: renaming temp file: %w", err)
	}
	return nil
}

// IsEncrypted returns true if data starts with the encryption version byte
// and is long enough to contain a valid encrypted payload.
func IsEncrypted(data []byte) bool {
	return len(data) >= MinEncryptedSize && data[0] == FileVersion
}
