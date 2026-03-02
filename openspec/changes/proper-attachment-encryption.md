# Proper Attachment Encryption (Layer 2)

## Goal

Ensure all email attachments stored on disk are encrypted using AES-256-GCM. This includes encrypting new attachments during synchronization and decrypting them during export or viewing.

## Problem Statement

The current implementation of "Encryption at Rest" only encrypts existing attachments during a one-time migration. New attachments added during sync remain in plaintext, and export commands fail to decrypt previously encrypted files.

## Proposed Changes

### 1. Synchronizer Integration (`internal/sync/sync.go`)
- Update `storeAttachment` to encrypt content before writing to disk if encryption is enabled in the configuration.
- Use `encryption.EncryptBytes` to generate the encrypted payload.
- Ensure the content-addressed hash (SHA-256) remains based on the *plaintext* content to preserve deduplication across encrypted and unencrypted stores.

### 2. Export Command Integration (`cmd/msgvault/cmd/export_*.go`)
- Update `export-attachment` and `export-attachments` to detect if a file on disk is encrypted using `encryption.IsEncrypted()`.
- If encrypted, retrieve the master key from the provider and use `encryption.DecryptBytes` (or a streaming equivalent) before writing to the destination.

### 3. TUI & API Integration
- Ensure the TUI's "Open" or "Export" actions correctly decrypt attachments before handing them to the OS or user.
- Update the MCP (Model Context Protocol) server to decrypt attachments before serving them to AI clients.

## Verification

### Integration Tests
- `TestSync_EncryptedAttachments`: Verify that a sync run with encryption enabled produces encrypted files in the attachments directory.
- `TestExport_EncryptedAttachments`: Verify that `export-attachment` correctly restores the original plaintext file from an encrypted source.
- `TestDeduplication_Encrypted`: Verify that two messages with the same attachment result in only one encrypted file on disk.

### Manual Verification
- Run `msgvault sync-full`.
- Verify `file ~/.msgvault/attachments/xx/xxxx...` reports "data" (encrypted) instead of the original file type.
- Run `msgvault export-attachment <hash> -o test.pdf`.
- Verify `test.pdf` is a valid, readable PDF.
