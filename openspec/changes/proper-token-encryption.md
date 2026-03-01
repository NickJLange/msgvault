# Proper Token Encryption (Layer 2)

## Goal

Protect OAuth access and refresh tokens stored on disk in the `tokens/` directory using AES-256-GCM encryption.

## Problem Statement

The `msgvault encrypt` command currently encrypts existing tokens, but the `oauth.Manager` continues to save new and refreshed tokens in plaintext. This leaves sensitive account credentials exposed on disk when new accounts are added or tokens are automatically refreshed during sync.

## Proposed Changes

### 1. Token Manager Integration (`internal/oauth/oauth.go`)
- Update `saveToken` to encrypt the JSON payload using `encryption.EncryptBytes` before writing to disk if encryption is enabled.
- Update `loadToken` and `loadTokenFile` to check for encryption with `encryption.IsEncrypted()`.
- If encrypted, retrieve the master key and decrypt with `encryption.DecryptBytes` before unmarshaling the JSON.

### 2. Provider Context
- The `oauth.Manager` should be updated to accept a `KeyProvider` or have its `loadToken`/`saveToken` methods updated to accept a context (for `GetKey`).

## Verification

### Integration Tests
- `TestOAuth_EncryptedTokenSave`: Verify that `saveToken` with encryption enabled writes an encrypted file.
- `TestOAuth_EncryptedTokenLoad`: Verify that `loadToken` correctly decrypts and parses an encrypted token file.
- `TestOAuth_AutoRefreshEncryption`: Verify that a token refreshed during sync is saved back to disk in encrypted form.

### Manual Verification
- Run `msgvault add-account you@gmail.com`.
- Verify the token file `~/.msgvault/tokens/you@gmail.com.json` is binary/encrypted (not readable JSON).
- Run `msgvault sync-incremental` and verify the token is correctly loaded and refreshed.
