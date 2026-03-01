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

		// Re-key the SQLCipher database
		if _, err := os.Stat(dbPath); err == nil {
			fmt.Println("Re-keying database...")
			if err := rekeyDatabase(dbPath, oldKey, newKey); err != nil {
				return fmt.Errorf("re-keying database: %w", err)
			}
			fmt.Println("  Database re-keyed successfully")
		}

		// Delete Parquet cache (will be rebuilt with new key on next TUI launch)
		analyticsDir := cfg.AnalyticsDir()
		if _, err := os.Stat(analyticsDir); err == nil {
			fmt.Println("Clearing Parquet cache (will rebuild on next TUI launch)...")
			if err := os.RemoveAll(analyticsDir); err != nil {
				logger.Warn("failed to clear analytics cache", "err", err)
			}
		}

		// Store new key in provider
		switch cfg.Encryption.Provider {
		case "keyring", "":
			p := encryption.NewKeyringProvider(dbPath)
			if err := p.SetKey(newKey); err != nil {
				return fmt.Errorf("storing new key in keyring: %w\n⚠️  DATABASE HAS BEEN RE-KEYED but new key was not stored.\nNew key fingerprint: %s\nExport it manually before it is lost", err, encryption.KeyFingerprint(newKey))
			}
		case "keyfile":
			path := cfg.Encryption.Keyfile.Path
			if path == "" {
				return fmt.Errorf("keyfile path not configured")
			}
			encoded := encodeKeyBase64(newKey)

			// Atomic write: temp file + rename
			dir := filepath.Dir(path)
			tmp, err := os.CreateTemp(dir, ".keyfile-*")
			if err != nil {
				return fmt.Errorf("creating temp keyfile: %w", err)
			}
			tmpPath := tmp.Name()

			if _, err := tmp.Write([]byte(encoded + "\n")); err != nil {
				tmp.Close()
				os.Remove(tmpPath)
				return fmt.Errorf("writing temp keyfile: %w", err)
			}
			if err := tmp.Chmod(0600); err != nil {
				tmp.Close()
				os.Remove(tmpPath)
				return fmt.Errorf("setting keyfile permissions: %w", err)
			}
			if err := tmp.Close(); err != nil {
				os.Remove(tmpPath)
				return fmt.Errorf("closing temp keyfile: %w", err)
			}
			if err := os.Rename(tmpPath, path); err != nil {
				os.Remove(tmpPath)
				return fmt.Errorf("writing new key to keyfile: %w", err)
			}
		default:
			// For env/exec providers, we can't store the key — user must update it externally
			fmt.Printf("\n⚠️  Provider %q is read-only. Update the key source with the new key.\n", cfg.Encryption.Provider)
			fmt.Printf("   New key (base64): %s\n", encodeKeyBase64(newKey))
		}

		fmt.Printf("\n✅ Key rotated successfully\n")
		fmt.Printf("   Old fingerprint: %s\n", encryption.KeyFingerprint(oldKey))
		fmt.Printf("   New fingerprint: %s\n", encryption.KeyFingerprint(newKey))
		fmt.Printf("\n⚠️  Back up your new key: msgvault key export --out ~/msgvault-key-backup.txt\n")

		return nil
	},
}

// rekeyDatabase changes the encryption key on a SQLCipher database by exporting
// to a new encrypted database and then swapping files.
func rekeyDatabase(dbPath string, oldKey, newKey []byte) error {
	newDBPath := dbPath + ".rotated"
	os.Remove(newDBPath) // Clean up any failed attempt

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
		strings.ReplaceAll(newDBPath, "'", "''"), newHex)
	if _, err := db.Exec(attachSQL); err != nil {
		return fmt.Errorf("attach new database: %w", err)
	}

	// Export all data to the new database
	if _, err := db.Exec("SELECT sqlcipher_export('new_db')"); err != nil {
		os.Remove(newDBPath)
		return fmt.Errorf("sqlcipher_export: %w", err)
	}

	// Copy WAL mode
	if _, err := db.Exec("PRAGMA new_db.journal_mode = WAL"); err != nil {
		logger.Warn("failed to set WAL on new database", "err", err)
	}

	// Detach
	if _, err := db.Exec("DETACH DATABASE new_db"); err != nil {
		os.Remove(newDBPath)
		return fmt.Errorf("detach new database: %w", err)
	}
	db.Close()

	// Swap files
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")
	if err := fileutil.AtomicRename(newDBPath, dbPath); err != nil {
		return fmt.Errorf("swap rotated database: %w", err)
	}
	os.Remove(newDBPath + "-wal")
	os.Remove(newDBPath + "-shm")

	return nil
}

// encodeKeyBase64 returns the base64 encoding of a key.
func encodeKeyBase64(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

func init() {
	keyCmd.AddCommand(rotateKeyCmd)
}
