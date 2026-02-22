# Encryption at Rest

## Goal

Encrypt all msgvault data at rest (SQLite database, Parquet cache, attachments, OAuth tokens) so that physical access to the storage medium does not expose email contents. The decryption key should come from a configurable source, including HashiCorp Vault.

## What Needs Encryption

| Data | Path | Sensitivity | Size |
|---|---|---|---|
| SQLite database | `msgvault.db` | **Critical** — messages, metadata, FTS index | 1–100+ GB |
| Message bodies | `message_bodies` table | **Critical** — full email text | Bulk of DB size |
| Raw MIME blobs | `message_raw` table (zlib) | **Critical** — complete original messages | Large |
| OAuth tokens | `tokens/*.json` | **Critical** — Gmail account access | Tiny |
| Attachments | `attachments/` | **High** — content-addressed files | Variable |
| Parquet cache | `analytics/` | **Medium** — denormalized metadata (no bodies) | Small |
| Config | `config.toml` | **Low** — may contain API keys | Tiny |

## Options Analysis

### Option A: SQLCipher (Database-Level Encryption)

**How**: Replace `mattn/go-sqlite3` with SQLCipher-enabled build. AES-256-CBC encryption of every database page, transparent to all queries.

| Aspect | Detail |
|---|---|
| Performance | 5–15% overhead (Zetetic benchmarks). Negligible for msgvault's read-heavy workload |
| Key delivery | `PRAGMA key = '...'` on connection open |
| Coverage | SQLite DB only — attachments, Parquet, tokens need separate encryption |
| CGo impact | Requires linking against SQLCipher instead of stock SQLite. Build complexity increases |
| Go driver | `CovenantSQL/go-sqlite3-encrypt` (fork of mattn) or link SQLCipher manually |
| Integrity | HMAC per page (tamper detection) enabled by default |
| Maturity | Production-proven, widely deployed, active maintenance |

**Verdict**: Strong for database encryption but requires swapping the SQLite driver and doesn't cover non-DB files.

### Option B: ncruces/go-sqlite3 Adiantum or XTS VFS

**How**: Use the `ncruces/go-sqlite3` driver with its encryption VFS. Adiantum (XChaCha12 + AES + Poly1305) or AES-XTS (NIST/FIPS approved). Encrypts all SQLite files at the VFS layer.

| Aspect | Detail |
|---|---|
| Performance | ~15% overhead measured on speedtest1 (Adiantum). AES-XTS comparable |
| Key delivery | Passed via URI parameter or API call when opening DB |
| Coverage | SQLite DB only — same limitation as SQLCipher |
| CGo impact | Uses Wasm-based SQLite (no CGo!) — simpler cross-compilation |
| Integrity | No MAC by default (Adiantum is deterministic). XTS also no MAC |
| Maturity | Newer, but well-designed. Active maintainer |

**Verdict**: Interesting for CGo-free builds but would require replacing both `mattn/go-sqlite3` and `go-duckdb` drivers. Too disruptive.

### Option C: Application-Level Envelope Encryption

**How**: Encrypt data before writing to SQLite/files. Use a Data Encryption Key (DEK) encrypted by a Key Encryption Key (KEK) from a configurable source. AES-256-GCM for symmetric encryption.

| Aspect | Detail |
|---|---|
| Performance | Minimal — AES-GCM is hardware-accelerated on all modern CPUs. ~1 GB/s on ARM64 |
| Key delivery | KEK from configurable provider (file, env var, Vault, KMS) |
| Coverage | **Complete** — can encrypt everything: DB fields, files, tokens |
| CGo impact | None — uses Go stdlib `crypto/aes` + `crypto/cipher` |
| Integrity | AES-GCM provides authenticated encryption (built-in tamper detection) |
| Trade-off | FTS5 cannot search encrypted body text; must decrypt-then-search or use encrypted index |

**Verdict**: Maximum coverage but breaks FTS5 full-text search on encrypted fields. Complex.

### Option D: Filesystem-Level Encryption (Recommended as Complement)

**How**: Use LUKS, fscrypt, or dm-crypt on the volume containing `~/.msgvault/`. Transparent to the application.

| Aspect | Detail |
|---|---|
| Performance | LUKS: ~2-5% overhead. fscrypt: 5-10% overhead. Near-zero for AES-NI hardware |
| Key delivery | OS-level (PAM, TPM, key file, or manual unlock) |
| Coverage | **Complete** — everything under the mount point is encrypted |
| CGo impact | None — transparent to application |
| Integrity | Depends on FS (LUKS+dm-integrity, ZFS native encryption has it) |
| Trade-off | Not application-controlled — relies on host OS. Doesn't work in all containers |

**Verdict**: Simplest, best performance, full coverage. But not application-controlled and doesn't satisfy "configurable key source" requirement alone.

## Recommended Approach: Hybrid (SQLCipher + File Encryption + Key Provider)

Combine database-level and file-level encryption with a pluggable key provider:

### Layer 1: SQLCipher for Database

- Swap `mattn/go-sqlite3` for SQLCipher-enabled build
- All database contents encrypted transparently (messages, FTS5 index, metadata)
- FTS5 continues to work — encryption is below the SQL layer
- `PRAGMA key` set on every connection open from the key provider

### Layer 2: AES-256-GCM for Files

- Encrypt attachments, OAuth tokens, and config at the application level
- Each file encrypted with a per-file DEK, wrapped by the master KEK
- Small header on each file: `[version][nonce][encrypted-DEK][ciphertext]`
- Parquet cache is **not encrypted** — it contains only aggregate metadata (sender, domain, label, date, size) and can be rebuilt from the encrypted DB. If needed, encrypt it too.

### Layer 3: Pluggable Key Provider

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
| `passphrase` | Interactive prompt → Argon2id KDF | Personal desktop use |
| `keyfile` | Read 256-bit key from file path | Automated/container deployments |
| `env` | `MSGVAULT_ENCRYPTION_KEY` env var | CI/CD, container orchestration |
| `vault` | HashiCorp Vault KV or Transit engine | Enterprise / high-security |
| `exec` | Run external command, read key from stdout | Custom integrations (KMS, 1Password CLI, etc.) |

### HashiCorp Vault Integration

Two modes of Vault integration:

**Mode 1 — KV Secrets (key retrieval)**:
- Store the DEK in Vault KV v2: `vault kv put secret/msgvault/key dek=<base64-key>`
- msgvault reads key at startup via `github.com/hashicorp/vault-client-go`
- Key caching in memory for the session lifetime
- Vault token via `VAULT_TOKEN`, token file, or AppRole auth

**Mode 2 — Transit Secrets Engine (envelope encryption)**:
- Vault never exposes the raw key; msgvault sends data to Vault for encrypt/decrypt
- Higher security (key never leaves Vault) but higher latency
- Best for wrapping the DEK, not for bulk data encryption
- `POST /transit/encrypt/msgvault` to wrap DEK, `POST /transit/decrypt/msgvault` to unwrap

**Recommendation**: Use **KV mode for key retrieval** (simpler, lower latency). Transit for wrapping the master key if maximum security is needed.

## Configuration

```toml
[encryption]
enabled = true
provider = "passphrase"  # passphrase | keyfile | env | vault | exec

[encryption.keyfile]
path = "/run/secrets/msgvault-key"

[encryption.vault]
address = "https://vault.example.com:8200"
path = "secret/data/msgvault/key"     # KV v2 path
field = "dek"                          # field name within the secret
auth_method = "token"                  # token | approle
# token read from VAULT_TOKEN env var or token_path
token_path = "/run/secrets/vault-token"
# For AppRole auth:
# role_id = "..."
# secret_id_path = "/run/secrets/vault-secret-id"

[encryption.exec]
command = "op read op://vault/msgvault/key"  # 1Password example
```

## Proposed Changes

| File | Change |
|---|---|
| `internal/encryption/provider.go` | New — `KeyProvider` interface + passphrase, keyfile, env providers |
| `internal/encryption/vault.go` | New — HashiCorp Vault KV + Transit provider |
| `internal/encryption/exec.go` | New — External command provider |
| `internal/encryption/file.go` | New — AES-256-GCM file encryption (attachments, tokens) |
| `internal/config/config.go` | Add `EncryptionConfig` struct |
| `internal/store/store.go` | SQLCipher PRAGMA key on connection open |
| `go.mod` | Add `github.com/hashicorp/vault-client-go` |
| `cmd/msgvault/cmd/init_db.go` | Prompt for encryption setup on first run |
| `cmd/msgvault/cmd/rotate_key.go` | New — Key rotation command (re-encrypt DEK with new KEK) |
| `Dockerfile` | SQLCipher build dependencies |

## Migration Path

Existing unencrypted databases must be migrated:

1. `msgvault encrypt` — one-time command to encrypt an existing database
   - Creates new encrypted DB via SQLCipher `ATTACH` + copy
   - Encrypts existing attachment files in-place
   - Encrypts token files
2. `msgvault decrypt` — reverse operation for export/migration
3. Encryption flag stored in DB metadata to prevent accidental unencrypted opens

## Network Impact

- **Vault provider**: Adds outbound HTTPS to Vault server address (must be added to container network whitelist)
- **All other providers**: No new network calls

## Verification

1. Create encrypted DB with passphrase provider — verify raw `msgvault.db` is unreadable
2. Open encrypted DB — verify all queries, FTS5, TUI work normally
3. Rotate key — verify DB remains accessible with new key
4. Vault provider — verify key retrieval from Vault KV v2
5. Performance benchmark: encrypted vs unencrypted sync + query
6. `strings msgvault.db` shows no plaintext email content
7. Attachment files are unreadable without key
8. Startup fails gracefully with wrong key (clear error, no corruption)
