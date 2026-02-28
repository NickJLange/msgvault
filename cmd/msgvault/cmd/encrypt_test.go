package cmd

import (
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mutecomm/go-sqlcipher/v4"
	"github.com/wesm/msgvault/internal/encryption"
)

// TestEncryptDecryptDatabase tests the full migration path:
// 1. Create unencrypted DB with data
// 2. Encrypt it with encryptDatabase()
// 3. Verify plain sqlite3 cannot read it
// 4. Verify encrypted open with correct key works
// 5. Decrypt it with decryptDatabase()
// 6. Verify plain sqlite3 can read it again
func TestEncryptDecryptDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	key, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Step 1: Create unencrypted DB with schema and data
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE sources (id INTEGER PRIMARY KEY, identifier TEXT NOT NULL);
		CREATE TABLE messages (id INTEGER PRIMARY KEY, source_id INTEGER, subject TEXT, sent_at TIMESTAMP);
		INSERT INTO sources VALUES (1, 'test@gmail.com');
		INSERT INTO messages VALUES (1, 1, 'Hello World', '2024-01-15 10:00:00');
		INSERT INTO messages VALUES (2, 1, 'Secret Data', '2024-01-16 11:00:00');
	`)
	if err != nil {
		t.Fatalf("create schema/data: %v", err)
	}
	db.Close()

	// Step 2: Encrypt the database
	if err := encryptDatabase(dbPath, key); err != nil {
		t.Fatalf("encryptDatabase: %v", err)
	}

	// Step 3: Verify plaintext sqlite3 cannot read it
	plainDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open plain db: %v", err)
	}
	_, err = plainDB.Exec("SELECT * FROM sources")
	if err == nil {
		t.Fatal("expected error reading encrypted DB without key")
	}
	plainDB.Close()

	// Step 4: Verify the file doesn't leak plaintext
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read db file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "Hello World") || strings.Contains(content, "Secret Data") || strings.Contains(content, "test@gmail.com") {
		t.Error("encrypted database file contains plaintext data")
	}

	// Step 5: Verify encrypted open with correct key works
	encDB, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_pragma_key=x'"+hex.EncodeToString(key)+"'")
	if err != nil {
		t.Fatalf("open encrypted db: %v", err)
	}
	var count int
	if err := encDB.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count); err != nil {
		t.Fatalf("query encrypted db: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 messages, got %d", count)
	}
	var subject string
	if err := encDB.QueryRow("SELECT subject FROM messages WHERE id = 1").Scan(&subject); err != nil {
		t.Fatalf("query subject: %v", err)
	}
	if subject != "Hello World" {
		t.Errorf("subject = %q, want %q", subject, "Hello World")
	}
	encDB.Close()

	// Step 6: Decrypt the database
	if err := decryptDatabase(dbPath, key); err != nil {
		t.Fatalf("decryptDatabase: %v", err)
	}

	// Step 7: Verify plaintext sqlite3 can read it again
	plainDB2, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open decrypted db: %v", err)
	}
	defer plainDB2.Close()

	if err := plainDB2.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count); err != nil {
		t.Fatalf("query decrypted db: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 messages after decrypt, got %d", count)
	}
	if err := plainDB2.QueryRow("SELECT subject FROM messages WHERE id = 2").Scan(&subject); err != nil {
		t.Fatalf("query subject after decrypt: %v", err)
	}
	if subject != "Secret Data" {
		t.Errorf("subject = %q, want %q", subject, "Secret Data")
	}
}

// TestEncryptDatabase_NoTempFileLeaks verifies temp files are cleaned up.
func TestEncryptDatabase_NoTempFileLeaks(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	key, _ := encryption.GenerateKey()

	// Create a simple DB
	db, _ := sql.Open("sqlite3", dbPath)
	db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY); INSERT INTO t VALUES (1);")
	db.Close()

	if err := encryptDatabase(dbPath, key); err != nil {
		t.Fatalf("encryptDatabase: %v", err)
	}

	// Verify no temp files remain
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		name := e.Name()
		if name != "test.db" && name != "test.db-wal" && name != "test.db-shm" {
			t.Errorf("unexpected temp file remaining: %s", name)
		}
	}
}
