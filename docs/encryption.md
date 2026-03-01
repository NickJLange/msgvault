# Encryption at Rest

msgvault encrypts its core SQLite database at rest using SQLCipher, so that physical access to the storage medium does not expose email metadata, bodies, or the search index.

## What Gets Encrypted

| Data | Method | Details |
|---|---|---|
| SQLite database | SQLCipher (AES-256-CBC) | All tables, FTS5 index, metadata. Transparent to queries. |
| Message bodies | SQLCipher | Included in database encryption. |
| Raw MIME blobs | SQLCipher | Included in database encryption. |
| Parquet cache | DuckDB Modular Encryption | Analytics cache files encrypted with AES-256-GCM. |

*Note: Attachment and OAuth token file encryption are planned for a future update.*

## Quick Start

### Encrypt an existing database

```bash
# Generate key (stored in OS keychain) and encrypt the database
msgvault encrypt
```

This command:
1. Generates a 256-bit encryption key (if none exists)
2. Stores the key in your OS keychain
3. Migrates the SQLite database to SQLCipher via `sqlcipher_export()`
4. Updates `config.toml` to enable encryption

After encrypting, back up your key immediately:

```bash
msgvault key export --out ~/msgvault-key-backup.txt
```

### Decrypt for export or migration

```bash
msgvault decrypt
```

This reverses the encryption, restoring the SQLite database to its original unencrypted state.

## Key Providers

The encryption key comes from a configurable provider. Set the provider in `config.toml`:

```toml
[encryption]
enabled = true
provider = "keyring"  # keyring | keyfile | env | exec
```

### keyring (default)

Stores the key in your OS keychain. Zero friction — no passphrase to type, no key file to manage.

| OS | Backend |
|---|---|
| macOS | Keychain Services |
| Linux | Secret Service D-Bus API (GNOME Keyring, KWallet) |
| Windows | Credential Manager |

Each database gets an independent key, scoped by database path.

### keyfile

Reads a base64-encoded 256-bit key from a file. Good for automated and container deployments.

```toml
[encryption.keyfile]
path = "/run/secrets/msgvault-key"
```

### env

Reads a base64-encoded key from the `MSGVAULT_ENCRYPTION_KEY` environment variable.

### exec

Runs an external command and reads the base64-encoded key from stdout. Use this to integrate with password managers (like 1Password `op`), cloud KMS, or custom key retrieval.

```toml
[encryption.exec]
command = "op read op://vault/msgvault/key"
```

## Key Management

### Export (backup) a key

```bash
msgvault key export --out ~/msgvault-key-backup.txt
```

**Treat the exported key like a password.** Store it in a password manager or safe.

### Import a key

```bash
msgvault key import --from ~/msgvault-key-backup.txt
```

### Rotate the key

```bash
msgvault key rotate
```

This generates a new key and re-encrypts the SQLCipher database using a transaction-safe export-and-swap method. The Parquet cache is deleted and rebuilt on the next TUI launch.

### Verify key fingerprint

Compare fingerprints across machines without exposing the key:

```bash
msgvault key fingerprint
# SHA-256: a1b2c3d4e5f6a7b8
```

## How It Works

### Database encryption (SQLCipher)

msgvault uses [SQLCipher](https://www.zetetic.net/sqlcipher/) via the `mutecomm/go-sqlcipher/v4` driver — a drop-in replacement for `mattn/go-sqlite3` that adds transparent AES-256-CBC encryption of every database page.

- The key is passed via `_pragma_key` in the DSN on every connection open.
- FTS5 full-text search works unchanged (encryption is below the SQL layer).
- HMAC per page provides tamper detection.

### DuckDB sqlite_scanner limitation

DuckDB's `sqlite_scanner` extension cannot read SQLCipher-encrypted databases. When encryption is enabled, msgvault automatically uses a CSV fallback path: it reads the SQLite database using Go's SQLCipher driver, exports tables to temporary CSV files, and imports them into DuckDB. This is transparent — all aggregate queries work the same way.

## FAQ

### What happens with the wrong key?

Opening an encrypted database with the wrong key fails immediately. No data is corrupted; the database is intact, just inaccessible.

### What's the performance impact?

- **Inserts**: ~4.6x overhead on bulk inserts due to SQLCipher's strict WAL synchronization requirements compared to standard SQLite.
- **Queries**: Negligible (~1%) overhead for typical read queries.
- **Cache Build**: Parquet cache encryption adds ~2.5x overhead to the one-time cache building process.
