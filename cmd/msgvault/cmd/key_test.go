package cmd

import (
	"context"
	"encoding/base64"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/wesm/msgvault/internal/config"
	"github.com/wesm/msgvault/internal/encryption"
	"github.com/zalando/go-keyring"
)

func init() {
	keyring.MockInit()
}

// setupKeyTest creates a temp environment for key management tests.
func setupKeyTest(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	origCfg := cfg
	origLogger := logger

	cfg = config.NewDefaultConfig()
	cfg.HomeDir = tmpDir
	cfg.Data.DataDir = tmpDir
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	return tmpDir, func() {
		cfg = origCfg
		logger = origLogger
	}
}

// testCmd creates a minimal *cobra.Command with a background context for testing RunE functions.
func testCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())
	return cmd
}

func TestKeyInit_Keyring(t *testing.T) {
	_, cleanup := setupKeyTest(t)
	defer cleanup()

	cmd := testCmd()
	cmd.Flags().String("provider", "", "")

	err := keyInitCmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("key init: %v", err)
	}

	// Verify key was stored
	p := encryption.NewKeyringProvider(cfg.DatabaseDSN())
	key, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey after init: %v", err)
	}
	if len(key) != encryption.KeySize {
		t.Errorf("key size = %d, want %d", len(key), encryption.KeySize)
	}

	// Config should have encryption enabled
	if !cfg.Encryption.Enabled {
		t.Error("Encryption.Enabled should be true after init")
	}
}

func TestKeyInit_AlreadyExists(t *testing.T) {
	_, cleanup := setupKeyTest(t)
	defer cleanup()

	cmd := testCmd()
	cmd.Flags().String("provider", "", "")

	// First init should succeed
	if err := keyInitCmd.RunE(cmd, nil); err != nil {
		t.Fatalf("first key init: %v", err)
	}

	// Second init should fail
	if err := keyInitCmd.RunE(cmd, nil); err == nil {
		t.Fatal("second key init should fail")
	}
}

func TestKeyInit_Keyfile(t *testing.T) {
	tmpDir, cleanup := setupKeyTest(t)
	defer cleanup()

	keyPath := filepath.Join(tmpDir, "test-key.txt")
	cfg.Encryption.Keyfile.Path = keyPath

	cmd := testCmd()
	cmd.Flags().String("provider", "keyfile", "")

	err := keyInitCmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("key init --provider keyfile: %v", err)
	}

	// Verify key file was created
	data, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("ReadFile key: %v", err)
	}
	encoded := strings.TrimSpace(string(data))
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}
	if len(key) != encryption.KeySize {
		t.Errorf("key size = %d, want %d", len(key), encryption.KeySize)
	}
}

func TestKeyExportImport_Roundtrip(t *testing.T) {
	tmpDir, cleanup := setupKeyTest(t)
	defer cleanup()

	// Init key
	initCmd := testCmd()
	initCmd.Flags().String("provider", "", "")
	if err := keyInitCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("key init: %v", err)
	}

	// Get fingerprint of original key
	p := encryption.NewKeyringProvider(cfg.DatabaseDSN())
	originalKey, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	originalFP := encryption.KeyFingerprint(originalKey)

	// Export key
	exportPath := filepath.Join(tmpDir, "exported-key.txt")
	exportCmd := testCmd()
	exportCmd.Flags().String("out", exportPath, "")
	exportCmd.Flags().Bool("stdout", false, "")
	if err := keyExportCmd.RunE(exportCmd, nil); err != nil {
		t.Fatalf("key export: %v", err)
	}

	// Verify export file exists
	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("export file missing: %v", err)
	}

	// Delete key from keyring
	if err := p.DeleteKey(); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}

	// Import key
	importCmd := testCmd()
	importCmd.Flags().String("from", exportPath, "")
	importCmd.Flags().Bool("stdin", false, "")
	importCmd.Flags().String("provider", "", "")
	importCmd.Flags().String("keyfile-path", "", "")
	if err := keyImportCmd.RunE(importCmd, nil); err != nil {
		t.Fatalf("key import: %v", err)
	}

	// Verify imported key has same fingerprint
	importedKey, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey after import: %v", err)
	}
	importedFP := encryption.KeyFingerprint(importedKey)
	if importedFP != originalFP {
		t.Errorf("fingerprint mismatch: got %s, want %s", importedFP, originalFP)
	}
}

func TestKeyFingerprint_Consistent(t *testing.T) {
	_, cleanup := setupKeyTest(t)
	defer cleanup()

	// Init key
	initCmd := testCmd()
	initCmd.Flags().String("provider", "", "")
	if err := keyInitCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("key init: %v", err)
	}

	p := encryption.NewKeyringProvider(cfg.DatabaseDSN())
	key, err := p.GetKey(context.Background())
	if err != nil {
		t.Fatalf("GetKey: %v", err)
	}
	fp1 := encryption.KeyFingerprint(key)
	fp2 := encryption.KeyFingerprint(key)
	if fp1 != fp2 {
		t.Errorf("fingerprints differ: %s vs %s", fp1, fp2)
	}
}

func TestKeyExport_Stdout(t *testing.T) {
	_, cleanup := setupKeyTest(t)
	defer cleanup()

	// Init key
	initCmd := testCmd()
	initCmd.Flags().String("provider", "", "")
	if err := keyInitCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("key init: %v", err)
	}

	// Export to stdout (just verify no error — stdout capture is complex)
	exportCmd := testCmd()
	exportCmd.Flags().String("out", "", "")
	exportCmd.Flags().Bool("stdout", true, "")
	if err := keyExportCmd.RunE(exportCmd, nil); err != nil {
		t.Fatalf("key export --stdout: %v", err)
	}
}

func TestKeyExport_NoFlags(t *testing.T) {
	_, cleanup := setupKeyTest(t)
	defer cleanup()

	// Init key
	initCmd := testCmd()
	initCmd.Flags().String("provider", "", "")
	if err := keyInitCmd.RunE(initCmd, nil); err != nil {
		t.Fatalf("key init: %v", err)
	}

	// Export with no flags should error
	exportCmd := testCmd()
	exportCmd.Flags().String("out", "", "")
	exportCmd.Flags().Bool("stdout", false, "")
	if err := keyExportCmd.RunE(exportCmd, nil); err == nil {
		t.Fatal("key export with no flags should fail")
	}
}

func TestKeyImport_NoSource(t *testing.T) {
	_, cleanup := setupKeyTest(t)
	defer cleanup()

	importCmd := testCmd()
	importCmd.Flags().String("from", "", "")
	importCmd.Flags().Bool("stdin", false, "")
	importCmd.Flags().String("provider", "", "")
	importCmd.Flags().String("keyfile-path", "", "")
	if err := keyImportCmd.RunE(importCmd, nil); err == nil {
		t.Fatal("key import with no source should fail")
	}
}
