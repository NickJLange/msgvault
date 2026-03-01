package cmd

import (
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mutecomm/go-sqlcipher/v4"
	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/encryption"
	"github.com/wesm/msgvault/internal/fileutil"
)

var rotateKeyCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Rotate the encryption key",
	Long: `Generate a new encryption key and re-encrypt the database.

This command:
  1. Retrieves the current encryption key
  2. Generates a new 256-bit key
  3. Re-keys the SQLCipher database (sqlcipher_export)
  4. Stores the new key in the configured provider
  5. Deletes the Parquet cache (rebuild with new key on next TUI launch)

The old key is no longer valid after rotation.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := MustBeLocal("key rotate"); err != nil {
			return err
		}

		if !cfg.Encryption.Enabled {
			return fmt.Errorf("encryption is not enabled; run 'msgvault encrypt' first")
		}

		dbPath := cfg.DatabaseDSN()

		// Get current key
		provider, err := encryption.NewProvider(cfg.Encryption, dbPath)
		if err != nil {
			return fmt.Errorf("creating key provider: %w", err)
		}
		oldKey, err := provider.GetKey(cmd.Context())
		if err != nil {
			return fmt.Errorf("retrieving current key: %w", err)
		}

		fmt.Printf("Current key fingerprint: %s\n", encryption.KeyFingerprint(oldKey))

		// Generate new key
		newKey, err := encryption.GenerateKey()
		if err != nil {
			return fmt.Errorf("generating new key: %w", err)
		}

		// Re-key the SQLCipher database to a temporary file
		newDBPath := dbPath + ".rotated"
		if _, err := os.Stat(dbPath); err == nil {
			fmt.Println("Re-keying database to temporary file...")
			if err := rekeyDatabase(dbPath, newDBPath, oldKey, newKey); err != nil {
				return fmt.Errorf("re-keying database: %w", err)
			}
		}

		// Store new key in provider BEFORE swapping the database file.
		// This ensures we don't have a new-key database without the key.
		fmt.Println("Storing new key in provider...")
		switch cfg.Encryption.Provider {
		case "keyring", "":
			p := encryption.NewKeyringProvider(dbPath)
			if err := p.SetKey(newKey); err != nil {
				os.Remove(newDBPath)
				return fmt.Errorf("storing new key in keyring: %w", err)
			}
		case "keyfile":
			path := cfg.Encryption.Keyfile.Path
			if path == "" {
				os.Remove(newDBPath)
				return fmt.Errorf("keyfile path not configured")
			}
			encoded := encodeKeyBase64(newKey)

			// Atomic write: temp file + rename
			dir := filepath.Dir(path)
			tmp, err := os.CreateTemp(dir, ".keyfile-*")
			if err != nil {
				os.Remove(newDBPath)
				return fmt.Errorf("creating temp keyfile: %w", err)
			}
			tmpPath := tmp.Name()

			if _, err := tmp.Write([]byte(encoded + "\n")); err != nil {
				tmp.Close()
				os.Remove(tmpPath)
				os.Remove(newDBPath)
				return fmt.Errorf("writing temp keyfile: %w", err)
			}
			if err := tmp.Chmod(0600); err != nil {
				tmp.Close()
				os.Remove(tmpPath)
				os.Remove(newDBPath)
				return fmt.Errorf("setting keyfile permissions: %w", err)
			}
			if err := tmp.Close(); err != nil {
				os.Remove(tmpPath)
				os.Remove(newDBPath)
				return fmt.Errorf("closing temp keyfile: %w", err)
			}
			if err := os.Rename(tmpPath, path); err != nil {
				os.Remove(tmpPath)
				os.Remove(newDBPath)
				return fmt.Errorf("writing new key to keyfile: %w", err)
			}
		default:
			// For env/exec providers, we can't store the key — user must update it externally
			fmt.Printf("\n⚠️  Provider %q is read-only. Update the key source with the new key.\n", cfg.Encryption.Provider)
			fmt.Printf("   New key (base64): %s\n", encodeKeyBase64(newKey))
			fmt.Printf("   Press Enter once you have updated the key source to finalize the database swap...")
			fmt.Scanln()
		}

		// Now swap the database files
		if _, err := os.Stat(newDBPath); err == nil {
			fmt.Println("Finalizing database swap...")
			os.Remove(dbPath + "-wal")
			os.Remove(dbPath + "-shm")
			if err := fileutil.AtomicRename(newDBPath, dbPath); err != nil {
				return fmt.Errorf("swap rotated database: %w (your new database is at %s)", err, newDBPath)
			}
			os.Remove(dbPath + "-wal")
			os.Remove(dbPath + "-shm")
			fmt.Println("  Database re-keyed successfully")
		}

		// Delete Parquet cache
...
// rekeyDatabase changes the encryption key on a SQLCipher database by exporting
// to a new encrypted database at dstPath. It does NOT swap the files.
func rekeyDatabase(dbPath, dstPath string, oldKey, newKey []byte) error {
	os.Remove(dstPath) // Clean up any failed attempt

	oldHex := hex.EncodeToString(oldKey)
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON&_pragma_key=x'%s'", dbPath, oldHex)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("cannot read database (wrong key?): %w", err)
	}

	// Attach the new database with the new key
	newHex := hex.EncodeToString(newKey)
	attachSQL := fmt.Sprintf("ATTACH DATABASE '%s' AS new_db KEY \"x'%s'\"",
		strings.ReplaceAll(dstPath, "'", "''"), newHex)
	if _, err := db.Exec(attachSQL); err != nil {
		return fmt.Errorf("attach new database: %w", err)
	}

	// Export all data to the new database
	if _, err := db.Exec("SELECT sqlcipher_export('new_db')"); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("sqlcipher_export: %w", err)
	}

	// Copy WAL mode
	if _, err := db.Exec("PRAGMA new_db.journal_mode = WAL"); err != nil {
		logger.Warn("failed to set WAL on new database", "err", err)
	}

	// Detach
	if _, err := db.Exec("DETACH DATABASE new_db"); err != nil {
		os.Remove(dstPath)
		return fmt.Errorf("detach new database: %w", err)
	}
	db.Close()

	return nil
}

// encodeKeyBase64 returns the base64 encoding of a key.
func encodeKeyBase64(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

func init() {
	keyCmd.AddCommand(rotateKeyCmd)
}
