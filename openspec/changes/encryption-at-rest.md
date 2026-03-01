# Encryption at Rest (Layer 1: Database)

## Goal

Encrypt the core msgvault SQLite database at rest using SQLCipher so that physical access to the database file does not expose email metadata, bodies, or the search index. Establish the pluggable key provider infrastructure for all encryption layers.

## Scope (Layer 1)

| Data | Path | Sensitivity | Size |
|---|---|---|---|
| SQLite database | `msgvault.db` | **Critical** — messages, metadata, FTS index | 1–100+ GB |
| Message bodies | `message_bodies` table | **Critical** — full email text | Bulk of DB size |
| Raw MIME blobs | `message_raw` table (zlib) | **Critical** — complete original messages | Large |

*Note: Attachments and OAuth tokens are handled in separate feature specs.*

## Recommended Approach: SQLCipher + Pluggable Key Provider

### Layer 1: SQLCipher for Database

- Swap `mattn/go-sqlite3` for SQLCipher-enabled build (`mutecomm/go-sqlcipher/v4`)
- All database contents encrypted transparently (messages, FTS5 index, metadata)
- FTS5 continues to work — encryption is below the SQL layer
- `PRAGMA key` set on every connection open from the key provider

### Pluggable Key Provider Infrastructure

```go
// KeyProvider retrieves the master encryption key (KEK).
type KeyProvider interface {
    // GetKey returns the key encryption key.
    GetKey(ctx context.Context) ([]byte, error)
    // Name returns the provider name for logging.
    Name() string
}
```

| Provider | Source | Use Case |
|---|---|---|
| `keyring` | OS keychain (macOS, GNOME, Windows) | **Default** — key stored securely by OS |
| `keyfile` | Read 256-bit key from file path | Automated/container deployments |
| `env` | `MSGVAULT_ENCRYPTION_KEY` env var | CI/CD, orchestration |
| `exec` | Run external command, read key from stdout | Custom integrations (KMS, 1Password) |

## Configuration

```toml
[encryption]
enabled = true
provider = "keyring"  # keyring | keyfile | env | exec

[encryption.keyfile]
path = "/run/secrets/msgvault-key"

[encryption.exec]
command = "op read op://vault/msgvault/key"
```

## Migration & Key Management

1. `msgvault encrypt` — migration command to encrypt existing DB via `ATTACH` + `sqlcipher_export()`.
2. `msgvault decrypt` — reverse migration to plaintext.
3. `msgvault key export/import/fingerprint` — backup and portability.
4. `msgvault key rotate` — transaction-safe re-keying using `sqlcipher_export()`.

## Network Impact

- None for core providers.

## Test Plan

- Unit tests for all `KeyProvider` implementations.
- Integration tests for SQLCipher open/read/write with correct and incorrect keys.
- Migration tests (plaintext -> encrypted and back).
- Key rotation tests (verify data integrity after rotation).
