package cmd

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mutecomm/go-sqlcipher/v4"
	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/encryption"
)

var encryptCmd = &cobra.Command{
	Use:   "encrypt",
	Short: "Encrypt existing data files",
	Long: `Encrypt existing data at rest using SQLCipher and AES-256-GCM.

This command encrypts:
  - SQLite database (SQLCipher with _pragma_key)
  - Attachment files (AES-256-GCM)
  - OAuth token files (AES-256-GCM)

If no encryption key exists, it generates one and stores it using the configured
provider (default: OS keyring).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := MustBeLocal("encrypt"); err != nil {
			return err
		}

		dbPath := cfg.DatabaseDSN()

		// Get or generate encryption key
		var key []byte
		provider := cfg.Encryption.Provider
		if provider == "" {
			provider = "keyring"
		}

		switch provider {
		case "keyring":
			p := encryption.NewKeyringProvider(dbPath)
			var err error
			key, err = p.GetKey(context.Background())
			if err != nil {
				if errors.Is(err, encryption.ErrKeyNotFound) {
					// No existing key — generate one
					key, err = encryption.GenerateKey()
					if err != nil {
						return fmt.Errorf("generating key: %w", err)
					}
					if err := p.SetKey(key); err != nil {
						return fmt.Errorf("storing key: %w", err)
					}
					fmt.Printf("🔑 Generated new encryption key (stored in OS keyring)\n")
				} else {
					return fmt.Errorf("retrieving key from keyring: %w", err)
				}
			}
		default:
			p, err := encryption.NewProvider(cfg.Encryption, dbPath)
			if err != nil {
				return fmt.Errorf("creating key provider: %w", err)
			}
			key, err = p.GetKey(cmd.Context())
			if err != nil {
				return fmt.Errorf("retrieving key: %w", err)
			}
		}

		// Encrypt SQLite database with SQLCipher
		if _, err := os.Stat(dbPath); err == nil {
			fmt.Println("Encrypting database with SQLCipher...")
			if err := encryptDatabase(dbPath, key); err != nil {
				return fmt.Errorf("encrypting database: %w", err)
			}
			fmt.Println("  Database encrypted successfully")
		}

		var filesEncrypted int

		// Encrypt token files
		tokensDir := cfg.TokensDir()
		if entries, err := os.ReadDir(tokensDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				path := filepath.Join(tokensDir, entry.Name())
				data, err := os.ReadFile(path)
				if err != nil {
					logger.Warn("skipping token file", "path", path, "err", err)
					continue
				}
				if encryption.IsEncrypted(data) {
					continue
				}
				if err := encryption.EncryptFile(key, path, path); err != nil {
					return fmt.Errorf("encrypting token %s: %w", entry.Name(), err)
				}
				filesEncrypted++
				logger.Info("encrypted token file", "path", path)
			}
		}

		// Encrypt attachment files
		attachDir := cfg.AttachmentsDir()
		if err := filepath.Walk(attachDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				logger.Warn("skipping attachment", "path", path, "err", readErr)
				return nil
			}
			if encryption.IsEncrypted(data) {
				return nil
			}
			if err := encryption.EncryptFile(key, path, path); err != nil {
				return fmt.Errorf("encrypting attachment %s: %w", path, err)
			}
			filesEncrypted++
			return nil
		}); err != nil {
			return fmt.Errorf("encrypting attachments: %w", err)
		}

		// Update config
		cfg.Encryption.Enabled = true
		if cfg.Encryption.Provider == "" {
			cfg.Encryption.Provider = provider
		}
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		fmt.Printf("✅ Encryption enabled (encrypted %d files)\n", filesEncrypted)
		fmt.Printf("   Fingerprint: %s\n", encryption.KeyFingerprint(key))
		fmt.Printf("\n⚠️  Back up your key: msgvault key export --out ~/msgvault-key-backup.txt\n")

		return nil
	},
}

var decryptCmd = &cobra.Command{
	Use:   "decrypt",
	Short: "Decrypt data files for export or migration",
	Long: `Decrypt data files that were encrypted by msgvault.

This command reverses the encryption applied by 'msgvault encrypt', restoring
the SQLite database, attachments, and tokens to their original unencrypted state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := MustBeLocal("decrypt"); err != nil {
			return err
		}

		if !cfg.Encryption.Enabled {
			return fmt.Errorf("encryption is not enabled")
		}

		provider, err := encryption.NewProvider(cfg.Encryption, cfg.DatabaseDSN())
		if err != nil {
			return fmt.Errorf("creating key provider: %w", err)
		}

		key, err := provider.GetKey(cmd.Context())
		if err != nil {
			return fmt.Errorf("retrieving key: %w", err)
		}

		// Decrypt SQLite database
		dbPath := cfg.DatabaseDSN()
		if _, err := os.Stat(dbPath); err == nil {
			fmt.Println("Decrypting database...")
			if err := decryptDatabase(dbPath, key); err != nil {
				return fmt.Errorf("decrypting database: %w", err)
			}
			fmt.Println("  Database decrypted successfully")
		}

		var filesDecrypted int

		// Decrypt token files
		tokensDir := cfg.TokensDir()
		if entries, err := os.ReadDir(tokensDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				path := filepath.Join(tokensDir, entry.Name())
				data, err := os.ReadFile(path)
				if err != nil {
					logger.Warn("skipping token file", "path", path, "err", err)
					continue
				}
				if !encryption.IsEncrypted(data) {
					continue
				}
				if err := encryption.DecryptFile(key, path, path); err != nil {
					return fmt.Errorf("decrypting token %s: %w", entry.Name(), err)
				}
				filesDecrypted++
				logger.Info("decrypted token file", "path", path)
			}
		}

		// Decrypt attachment files
		attachDir := cfg.AttachmentsDir()
		if err := filepath.Walk(attachDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				logger.Warn("skipping attachment", "path", path, "err", readErr)
				return nil
			}
			if !encryption.IsEncrypted(data) {
				return nil
			}
			if err := encryption.DecryptFile(key, path, path); err != nil {
				return fmt.Errorf("decrypting attachment %s: %w", path, err)
			}
			filesDecrypted++
			return nil
		}); err != nil {
			return fmt.Errorf("decrypting attachments: %w", err)
		}

		// Update config
		cfg.Encryption.Enabled = false
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		fmt.Printf("✅ Decrypted %d files\n", filesDecrypted)
		return nil
	},
}

// encryptDatabase migrates an unencrypted SQLite database to SQLCipher.
// Uses sqlcipher_export() to copy all data to an encrypted database, then swaps files.
func encryptDatabase(dbPath string, key []byte) error {
	encryptedPath := dbPath + ".encrypted"

	// Remove any leftover temp file from a previous failed attempt
	os.Remove(encryptedPath)

	// Open the unencrypted DB (no key)
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open unencrypted db: %w", err)
	}
	defer db.Close()

	// Verify we can read it (it should be unencrypted)
	if err := db.Ping(); err != nil {
		return fmt.Errorf("cannot read database (already encrypted?): %w", err)
	}

	// Attach the encrypted target with the key
	hexKey := hex.EncodeToString(key)
	attachSQL := fmt.Sprintf("ATTACH DATABASE '%s' AS encrypted KEY \"x'%s'\"",
		strings.ReplaceAll(encryptedPath, "'", "''"), hexKey)
	if _, err := db.Exec(attachSQL); err != nil {
		return fmt.Errorf("attach encrypted db: %w", err)
	}

	// Export all data to the encrypted database
	if _, err := db.Exec("SELECT sqlcipher_export('encrypted')"); err != nil {
		os.Remove(encryptedPath)
		return fmt.Errorf("sqlcipher_export: %w", err)
	}

	// Copy WAL mode and other pragmas to the encrypted db
	if _, err := db.Exec("PRAGMA encrypted.journal_mode = WAL"); err != nil {
		// Non-fatal: WAL will be set on next open via DSN params
		logger.Warn("failed to set WAL on encrypted db", "err", err)
	}

	// Detach
	if _, err := db.Exec("DETACH DATABASE encrypted"); err != nil {
		os.Remove(encryptedPath)
		return fmt.Errorf("detach encrypted db: %w", err)
	}
	db.Close()

	// Swap files: rename encrypted → original
	// Remove WAL/SHM files first (they belong to the old unencrypted DB)
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")
	if err := os.Rename(encryptedPath, dbPath); err != nil {
		return fmt.Errorf("swap encrypted db: %w", err)
	}
	// Clean up any WAL/SHM from the encrypted temp
	os.Remove(encryptedPath + "-wal")
	os.Remove(encryptedPath + "-shm")

	return nil
}

// decryptDatabase migrates a SQLCipher-encrypted database to unencrypted SQLite.
// Uses sqlcipher_export() to copy all data to a plaintext database, then swaps files.
func decryptDatabase(dbPath string, key []byte) error {
	plainPath := dbPath + ".decrypted"

	// Remove any leftover temp file from a previous failed attempt
	os.Remove(plainPath)

	// Open the encrypted DB with key
	hexKey := hex.EncodeToString(key)
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_pragma_key=x'%s'", dbPath, hexKey)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("open encrypted db: %w", err)
	}
	defer db.Close()

	// Verify we can read it with the key
	if err := db.Ping(); err != nil {
		return fmt.Errorf("cannot read database (wrong key?): %w", err)
	}

	// Attach the plaintext target (empty key = no encryption)
	attachSQL := fmt.Sprintf("ATTACH DATABASE '%s' AS plaintext KEY ''",
		strings.ReplaceAll(plainPath, "'", "''"))
	if _, err := db.Exec(attachSQL); err != nil {
		return fmt.Errorf("attach plaintext db: %w", err)
	}

	// Export all data to the plaintext database
	if _, err := db.Exec("SELECT sqlcipher_export('plaintext')"); err != nil {
		os.Remove(plainPath)
		return fmt.Errorf("sqlcipher_export: %w", err)
	}

	// Copy WAL mode
	if _, err := db.Exec("PRAGMA plaintext.journal_mode = WAL"); err != nil {
		logger.Warn("failed to set WAL on plaintext db", "err", err)
	}

	// Detach
	if _, err := db.Exec("DETACH DATABASE plaintext"); err != nil {
		os.Remove(plainPath)
		return fmt.Errorf("detach plaintext db: %w", err)
	}
	db.Close()

	// Swap files
	os.Remove(dbPath + "-wal")
	os.Remove(dbPath + "-shm")
	if err := os.Rename(plainPath, dbPath); err != nil {
		return fmt.Errorf("swap decrypted db: %w", err)
	}
	os.Remove(plainPath + "-wal")
	os.Remove(plainPath + "-shm")

	return nil
}

func init() {
	rootCmd.AddCommand(encryptCmd)
	rootCmd.AddCommand(decryptCmd)
}
