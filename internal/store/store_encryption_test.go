package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wesm/msgvault/internal/encryption"
	"github.com/wesm/msgvault/internal/store"
)

func TestOpenEncrypted(t *testing.T) {
	key, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "encrypted.db")
	st, err := store.OpenEncrypted(dbPath, key)
	if err != nil {
		t.Fatalf("OpenEncrypted: %v", err)
	}
	defer st.Close()

	if !st.IsEncrypted() {
		t.Error("IsEncrypted() = false, want true")
	}
	if len(st.EncryptionKey()) != encryption.KeySize {
		t.Errorf("EncryptionKey() len = %d, want %d", len(st.EncryptionKey()), encryption.KeySize)
	}

	// Schema should work normally
	if err := st.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	// Basic data operations should work
	source, err := st.GetOrCreateSource("gmail", "test@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateSource: %v", err)
	}
	if source.ID == 0 {
		t.Error("source ID should be non-zero")
	}

	stats, err := st.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.SourceCount != 1 {
		t.Errorf("SourceCount = %d, want 1", stats.SourceCount)
	}
}

func TestStore_IsEncrypted_False(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "plain.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	if st.IsEncrypted() {
		t.Error("IsEncrypted() = true for plain store, want false")
	}
	if st.EncryptionKey() != nil {
		t.Error("EncryptionKey() should be nil for plain store")
	}
}

func TestOpenEncrypted_WrongKey(t *testing.T) {
	key1, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key2, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Create encrypted DB with key1
	dbPath := filepath.Join(t.TempDir(), "encrypted.db")
	st, err := store.OpenEncrypted(dbPath, key1)
	if err != nil {
		t.Fatalf("OpenEncrypted: %v", err)
	}
	if err := st.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	st.Close()

	// Try to open with key2 — should fail
	_, err = store.OpenEncrypted(dbPath, key2)
	if err == nil {
		t.Fatal("expected error opening encrypted DB with wrong key")
	}
	if !strings.Contains(err.Error(), "wrong encryption key") {
		t.Errorf("expected 'wrong encryption key' error, got: %v", err)
	}
}

func TestOpenEncrypted_InvalidKeySize(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Too short
	_, err := store.OpenEncrypted(dbPath, []byte("too-short"))
	if err == nil {
		t.Fatal("expected error for short key")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("expected key size error, got: %v", err)
	}
}

func TestOpen_EncryptedDBRequiresKey(t *testing.T) {
	key, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Create an encrypted database
	dbPath := filepath.Join(t.TempDir(), "encrypted.db")
	st, err := store.OpenEncrypted(dbPath, key)
	if err != nil {
		t.Fatalf("OpenEncrypted: %v", err)
	}
	if err := st.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	// Insert some data
	_, err = st.GetOrCreateSource("gmail", "test@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateSource: %v", err)
	}
	st.Close()

	// Try to open the encrypted DB without a key — should fail
	_, err = store.Open(dbPath)
	if err == nil {
		t.Fatal("expected error opening encrypted DB without key")
	}
}

func TestOpenEncrypted_ReopenWithCorrectKey(t *testing.T) {
	key, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "encrypted.db")

	// Create and populate
	st, err := store.OpenEncrypted(dbPath, key)
	if err != nil {
		t.Fatalf("OpenEncrypted: %v", err)
	}
	if err := st.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	_, err = st.GetOrCreateSource("gmail", "roundtrip@example.com")
	if err != nil {
		t.Fatalf("GetOrCreateSource: %v", err)
	}
	st.Close()

	// Reopen with the same key
	st2, err := store.OpenEncrypted(dbPath, key)
	if err != nil {
		t.Fatalf("reopen OpenEncrypted: %v", err)
	}
	defer st2.Close()

	// Verify data survived
	stats, err := st2.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.SourceCount != 1 {
		t.Errorf("SourceCount = %d, want 1", stats.SourceCount)
	}
}
