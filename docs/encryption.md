# Encryption at Rest

msgvault encrypts all stored data at rest — the SQLite database, Parquet analytics cache, attachments, and OAuth tokens — so that physical access to the storage medium does not expose email contents.

## What Gets Encrypted

| Data | Method | Details |
|---|---|---|
| SQLite database | SQLCipher (AES-256-CBC) | All tables, FTS5 index, metadata. Transparent to queries |
| Message bodies | SQLCipher | Included in database encryption |
| Raw MIME blobs | SQLCipher | Included in database encryption |
| Parquet cache | DuckDB Modular Encryption | Analytics cache files encrypted with AES-256-GCM |
| Attachments | AES-256-GCM | Each file encrypted individually |
| OAuth tokens | AES-256-GCM | Each token file encrypted individually |

## Quick Start

### Encrypt an existing database

```bash
# Generate key (stored in OS keychain) and encrypt everything
msgvault encrypt
```

This command:
1. Generates a 256-bit encryption key (if none exists)
2. Stores the key in your OS keychain
3. Migrates the SQLite database to SQLCipher via `sqlcipher_export()`
4. Encrypts attachment files with AES-256-GCM
5. Encrypts OAuth token files with AES-256-GCM
6. Updates `config.toml` to enable encryption

After encrypting, back up your key immediately:

```bash
msgvault key export --out ~/msgvault-key-backup.txt
```

### Decrypt for export or migration

```bash
msgvault decrypt
```

This reverses the encryption, restoring all files to their original unencrypted state. Useful for migrating to another tool or debugging.

## Key Providers

The encryption key comes from a configurable provider. Set the provider in `config.toml`:

```toml
[encryption]
enabled = true
provider = "keyring"  # keyring | passphrase | keyfile | env | exec
```

### keyring (default)

Stores the key in your OS keychain. Zero friction — no passphrase to type, no key file to manage.

| OS | Backend |
|---|---|
| macOS | Keychain Services |
| Linux | Secret Service D-Bus API (GNOME Keyring, KWallet) |
| Windows | Credential Manager |

```toml
[encryption]
provider = "keyring"
```

Each database gets an independent key, scoped by database path.

### keyfile

Reads a base64-encoded 256-bit key from a file. Good for automated and container deployments.

```toml
[encryption]
provider = "keyfile"

[encryption.keyfile]
path = "/run/secrets/msgvault-key"
```

Generate a key file:

```bash
msgvault key init --provider keyfile
```

Or manually:

```bash
openssl rand -base64 32 > /run/secrets/msgvault-key
chmod 600 /run/secrets/msgvault-key
```

### env

Reads a base64-encoded key from the `MSGVAULT_ENCRYPTION_KEY` environment variable.

```toml
[encryption]
provider = "env"
```

```bash
export MSGVAULT_ENCRYPTION_KEY=$(openssl rand -base64 32)
msgvault encrypt
```

### passphrase

Derives a key from a passphrase using Argon2id (3 iterations, 64 MB memory, 4 threads). Requires interactive setup.

```toml
[encryption]
provider = "passphrase"
```

```bash
msgvault key init --provider passphrase
```

### exec

Runs an external command and reads the base64-encoded key from stdout. Use this to integrate with password managers, cloud KMS, or custom key retrieval.

```toml
[encryption]
provider = "exec"

[encryption.exec]
command = "op read op://vault/msgvault/key"  # 1Password example
```

Other examples:

```toml
# AWS Secrets Manager
command = "aws secretsmanager get-secret-value --secret-id msgvault-key --query SecretString --output text"

# GCP Secret Manager
command = "gcloud secrets versions access latest --secret=msgvault-key"

# Custom script
command = "/usr/local/bin/get-msgvault-key.sh"
```

## Key Management

### Initialize a key

```bash
# Store in OS keychain (default)
msgvault key init

# Store as key file
msgvault key init --provider keyfile
```

### Export (backup) a key

```bash
# To a file
msgvault key export --out ~/msgvault-key-backup.txt

# To stdout (for piping to a secret manager)
msgvault key export --stdout
```

**Treat the exported key like a password.** Store it in a password manager, safe, or secondary keychain.

### Import a key

```bash
# From a file
msgvault key import --from ~/msgvault-key-backup.txt

# From stdin
op read "op://vault/msgvault/key" | msgvault key import --stdin

# Import to a specific provider
msgvault key import --from ~/key.txt --provider keyfile --keyfile-path /run/secrets/msgvault-key
```

### Rotate the key

```bash
msgvault key rotate
```

This generates a new key and re-encrypts everything: the SQLCipher database (via `PRAGMA rekey`), all attachment files, and all token files. The Parquet cache is deleted and rebuilt on the next TUI launch. The old key is no longer valid after rotation.

### Verify key fingerprint

Compare fingerprints across machines without exposing the key:

```bash
msgvault key fingerprint
# SHA-256: a1b2c3d4e5f6a7b8
```

## Moving a Database

```bash
# Source machine
msgvault key export --out /tmp/msgvault-key.txt
cp ~/.msgvault/msgvault.db /mnt/nas/msgvault.db

# Destination machine
msgvault key import --from /tmp/msgvault-key.txt

# Securely delete the export
shred -u /tmp/msgvault-key.txt    # Linux
# rm -P /tmp/msgvault-key.txt     # macOS
```

Verify both machines use the same key:

```bash
msgvault key fingerprint
```

## How It Works

### Database encryption (SQLCipher)

msgvault uses [SQLCipher](https://www.zetetic.net/sqlcipher/) via the `mutecomm/go-sqlcipher/v4` Go driver — a drop-in replacement for `mattn/go-sqlite3` that adds transparent AES-256-CBC encryption of every database page.

- The key is passed via `_pragma_key` in the DSN on every connection open
- FTS5 full-text search works unchanged (encryption is below the SQL layer)
- HMAC per page provides tamper detection
- Typical overhead: 5–15%

### File encryption (AES-256-GCM)

Attachments and OAuth tokens are encrypted individually using AES-256-GCM with Go's standard library (`crypto/aes` + `crypto/cipher`).

File format: `[version: 1 byte][nonce: 12 bytes][ciphertext + GCM tag]`

- Random nonce per file (re-encrypting the same file produces different ciphertext)
- GCM provides authenticated encryption (tamper detection)
- Atomic writes via temp file + rename

### Parquet cache encryption

The analytics cache uses DuckDB's Parquet Modular Encryption (AES-256-GCM):

```sql
COPY (...) TO 'messages.parquet' (
    FORMAT PARQUET,
    ENCRYPTION_CONFIG {footer_key: 'msgvault_key'}
)
```

The Parquet encryption key is derived from the same master key used for SQLCipher.

### DuckDB sqlite_scanner limitation

DuckDB's `sqlite_scanner` extension cannot read SQLCipher-encrypted databases. When encryption is enabled, msgvault automatically uses a CSV fallback path: it reads the SQLite database using Go's SQLCipher driver, exports tables to temporary CSV files, and imports them into DuckDB. This is transparent — all queries work the same way.

## Container Deployments

For Docker or Kubernetes, use the `keyfile` or `env` provider:

```dockerfile
# Mount key as a secret
docker run -v /run/secrets/msgvault-key:/run/secrets/msgvault-key \
  -e MSGVAULT_HOME=/data \
  msgvault sync-full user@gmail.com
```

```toml
# config.toml
[encryption]
enabled = true
provider = "keyfile"

[encryption.keyfile]
path = "/run/secrets/msgvault-key"
```

Or with environment variables:

```bash
docker run -e MSGVAULT_ENCRYPTION_KEY=$(cat /run/secrets/key) \
  -e MSGVAULT_HOME=/data \
  msgvault sync-full user@gmail.com
```

```toml
[encryption]
enabled = true
provider = "env"
```

## FAQ

### What happens with the wrong key?

Opening an encrypted database with the wrong key fails immediately with a clear error message. No data is corrupted — the database is intact, just inaccessible.

### Can I recover data without the key?

No. This is by design. Always back up your key: `msgvault key export --out ~/key-backup.txt`

### Can I have multiple encrypted databases?

Yes. The keyring provider scopes keys by database path, so each database can have an independent key.

### Is the Parquet cache sensitive?

The Parquet cache contains denormalized metadata (sender addresses, domains, labels, dates, message sizes) but no message bodies. It's encrypted when database encryption is enabled, but it can also be safely deleted and rebuilt from the encrypted database with `msgvault build-cache --full-rebuild`.

### How do I check if my database is encrypted?

```bash
# This should fail with "file is not a database"
sqlite3 ~/.msgvault/msgvault.db "SELECT count(*) FROM messages;"

# This should show no email content
strings ~/.msgvault/msgvault.db | grep -i "subject:" | head
```

### What's the performance impact?

SQLCipher adds 5–15% overhead on database operations. AES-GCM file encryption is hardware-accelerated on modern CPUs (~1 GB/s on ARM64). The Parquet cache encryption adds ~2.5x overhead to cache builds, but cache builds are infrequent.
