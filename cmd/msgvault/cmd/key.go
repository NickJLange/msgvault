package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/encryption"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage encryption keys",
	Long: `Manage encryption keys for msgvault's encryption at rest.

Use subcommands to initialize, export, import, and inspect encryption keys.`,
}

// --- key init ---

var keyInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new encryption key",
	Long: `Generate a new encryption key and store it using the configured provider.

By default, the key is stored in the OS keychain (macOS Keychain, GNOME Keyring,
Windows Credential Manager). Use --provider to specify a different provider.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath := cfg.DatabaseDSN()
		provider := cfg.Encryption.Provider
		if flagProvider, _ := cmd.Flags().GetString("provider"); flagProvider != "" {
			provider = flagProvider
		}
		if provider == "" {
			provider = "keyring"
		}

		key, err := encryption.GenerateKey()
		if err != nil {
			return fmt.Errorf("generating key: %w", err)
		}

		switch provider {
		case "keyring":
			p := encryption.NewKeyringProvider(dbPath)
			// Check if key already exists
			if _, err := p.GetKey(context.Background()); err == nil {
				return fmt.Errorf("encryption key already exists in OS keyring for %s\n\nUse 'msgvault key rotate' to change the key", dbPath)
			}
			if err := p.SetKey(key); err != nil {
				return fmt.Errorf("storing key in OS keyring: %w", err)
			}
			fmt.Printf("🔑 Encryption key generated and stored in OS keyring\n")
			fmt.Printf("   Database: %s\n", dbPath)
			fmt.Printf("   Fingerprint: %s\n", encryption.KeyFingerprint(key))
			fmt.Printf("\n⚠️  Back up your key: msgvault key export --out ~/msgvault-key-backup.txt\n")
		case "keyfile":
			path := cfg.Encryption.Keyfile.Path
			if path == "" {
				return fmt.Errorf("keyfile provider requires [encryption.keyfile] path in config")
			}
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("encryption key already exists at %s\n\nUse 'msgvault key rotate' to change the key", path)
			}
			encoded := base64.StdEncoding.EncodeToString(key)
			if err := os.WriteFile(path, []byte(encoded+"\n"), 0600); err != nil {
				return fmt.Errorf("writing key file: %w", err)
			}
			fmt.Printf("🔑 Encryption key generated and saved to %s\n", path)
			fmt.Printf("   Fingerprint: %s\n", encryption.KeyFingerprint(key))
		default:
			return fmt.Errorf("key init only supports 'keyring' and 'keyfile' providers; got %q", provider)
		}

		// Enable encryption in config
		cfg.Encryption.Enabled = true
		cfg.Encryption.Provider = provider
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		return nil
	},
}

// --- key export ---

var keyExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the encryption key",
	Long: `Export the encryption key for backup purposes.

The key is output as a base64-encoded string. Store it securely — anyone
with this key can decrypt your database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		outPath, _ := cmd.Flags().GetString("out")
		toStdout, _ := cmd.Flags().GetBool("stdout")

		provider, err := encryption.NewProvider(cfg.Encryption, cfg.DatabaseDSN())
		if err != nil {
			return fmt.Errorf("creating key provider: %w", err)
		}

		key, err := provider.GetKey(cmd.Context())
		if err != nil {
			return fmt.Errorf("retrieving key: %w", err)
		}

		encoded := base64.StdEncoding.EncodeToString(key)

		if toStdout {
			fmt.Print(encoded)
			return nil
		}

		if outPath != "" {
			if err := os.WriteFile(outPath, []byte(encoded+"\n"), 0600); err != nil {
				return fmt.Errorf("writing key file: %w", err)
			}
			fmt.Printf("🔑 Key exported to %s\n", outPath)
			fmt.Printf("   Fingerprint: %s\n", encryption.KeyFingerprint(key))
			fmt.Printf("\n⚠️  Store this file securely and delete it after copying to a safe location.\n")
			return nil
		}

		return fmt.Errorf("specify --out <file> or --stdout")
	},
}

// --- key import ---

var keyImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import an encryption key",
	Long: `Import an encryption key from a backup file or stdin.

This stores the key using the configured provider (default: OS keyring).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fromPath, _ := cmd.Flags().GetString("from")
		fromStdin, _ := cmd.Flags().GetBool("stdin")
		toProvider, _ := cmd.Flags().GetString("provider")

		if fromPath == "" && !fromStdin {
			return fmt.Errorf("specify --from <file> or --stdin")
		}

		var encoded string
		if fromStdin {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			encoded = strings.TrimSpace(string(data))
		} else {
			data, err := os.ReadFile(fromPath)
			if err != nil {
				return fmt.Errorf("reading key file: %w", err)
			}
			encoded = strings.TrimSpace(string(data))
		}

		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return fmt.Errorf("decoding key: %w", err)
		}
		if err := encryption.ValidateKey(key); err != nil {
			return err
		}

		if toProvider == "" {
			toProvider = cfg.Encryption.Provider
		}
		if toProvider == "" {
			toProvider = "keyring"
		}

		dbPath := cfg.DatabaseDSN()

		switch toProvider {
		case "keyring":
			p := encryption.NewKeyringProvider(dbPath)
			if err := p.SetKey(key); err != nil {
				return fmt.Errorf("storing key: %w", err)
			}
			fmt.Printf("🔑 Key imported to OS keyring\n")
		case "keyfile":
			path := cfg.Encryption.Keyfile.Path
			if flagPath, _ := cmd.Flags().GetString("keyfile-path"); flagPath != "" {
				path = flagPath
			}
			if path == "" {
				return fmt.Errorf("keyfile provider requires path; use --keyfile-path or set [encryption.keyfile] path in config")
			}
			if err := os.WriteFile(path, []byte(encoded+"\n"), 0600); err != nil {
				return fmt.Errorf("writing key file: %w", err)
			}
			fmt.Printf("🔑 Key imported to %s\n", path)
		default:
			return fmt.Errorf("import only supports 'keyring' and 'keyfile' providers; got %q", toProvider)
		}

		fmt.Printf("   Fingerprint: %s\n", encryption.KeyFingerprint(key))

		// Enable encryption in config if not already
		if !cfg.Encryption.Enabled {
			cfg.Encryption.Enabled = true
			cfg.Encryption.Provider = toProvider
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
		}

		return nil
	},
}

// --- key fingerprint ---

var keyFingerprintCmd = &cobra.Command{
	Use:   "fingerprint",
	Short: "Show the encryption key fingerprint",
	Long: `Display a fingerprint of the current encryption key.

The fingerprint can be used to verify that two machines are using the same key
without exposing the key itself.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := encryption.NewProvider(cfg.Encryption, cfg.DatabaseDSN())
		if err != nil {
			return fmt.Errorf("creating key provider: %w", err)
		}

		key, err := provider.GetKey(cmd.Context())
		if err != nil {
			return fmt.Errorf("retrieving key: %w", err)
		}

		fmt.Println(encryption.KeyFingerprint(key))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(keyCmd)

	keyInitCmd.Flags().String("provider", "", "key provider (keyring, keyfile)")
	keyCmd.AddCommand(keyInitCmd)

	keyExportCmd.Flags().String("out", "", "output file path")
	keyExportCmd.Flags().Bool("stdout", false, "write key to stdout")
	keyCmd.AddCommand(keyExportCmd)

	keyImportCmd.Flags().String("from", "", "key file to import")
	keyImportCmd.Flags().Bool("stdin", false, "read key from stdin")
	keyImportCmd.Flags().String("provider", "", "target provider (keyring, keyfile)")
	keyImportCmd.Flags().String("keyfile-path", "", "path for keyfile provider")
	keyCmd.AddCommand(keyImportCmd)

	keyCmd.AddCommand(keyFingerprintCmd)
}
