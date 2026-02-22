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
| `keyring` | OS keychain (macOS Keychain, GNOME Keyring, KWallet, Windows Credential Manager) | **Default for desktop use** — key stored securely by OS |
| `passphrase` | Interactive prompt → Argon2id KDF | Fallback when no OS keyring available |
| `keyfile` | Read 256-bit key from file path | Automated/container deployments |
| `env` | `MSGVAULT_ENCRYPTION_KEY` env var | CI/CD, container orchestration |
| `vault` | HashiCorp Vault KV or Transit engine | Enterprise / high-security |
| `exec` | Run external command, read key from stdout | Custom integrations (KMS, 1Password CLI, etc.) |

### OS Keyring Provider (Default)

Uses [`zalando/go-keyring`](https://github.com/zalando/go-keyring) (1.1k stars, pure Go, no CGo):

| OS | Backend | Notes |
|---|---|---|
| macOS | Keychain Services (via `/usr/bin/security`) | Protected by login password + Secure Enclave on Apple Silicon |
| Linux | Secret Service D-Bus API (GNOME Keyring, KWallet) | Unlocked with user login session |
| Windows | Credential Manager | Protected by user login |

**Workflow**:
1. On first `msgvault encrypt` or `init-db --encrypted`, generate a random 256-bit key
2. Store it in the OS keyring: `keyring.Set("msgvault", "<db-path>", base64(key))`
3. On every subsequent open, retrieve: `keyring.Get("msgvault", "<db-path>")`
4. If keyring unavailable (headless server, container), fall back to configured provider

**Benefits**:
- Zero-friction for desktop users — no passphrase to type, no key file to manage
- Key protected by OS-level security (login password, biometrics on macOS)
- Multiple databases can have independent keys (keyed by DB path)
- Key persists across reboots without user intervention

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
provider = "keyring"  # keyring | passphrase | keyfile | env | vault | exec

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
| `internal/encryption/provider.go` | New — `KeyProvider` interface + keyring, passphrase, keyfile, env providers |
| `internal/encryption/keyring.go` | New — OS keyring provider via `zalando/go-keyring` |
| `internal/encryption/vault.go` | New — HashiCorp Vault KV + Transit provider |
| `internal/encryption/exec.go` | New — External command provider |
| `internal/encryption/file.go` | New — AES-256-GCM file encryption (attachments, tokens) |
| `internal/config/config.go` | Add `EncryptionConfig` struct |
| `internal/store/store.go` | SQLCipher PRAGMA key on connection open |
| `go.mod` | Add `github.com/zalando/go-keyring`, `github.com/hashicorp/vault-client-go` |
| `cmd/msgvault/cmd/init_db.go` | Prompt for encryption setup on first run |
| `cmd/msgvault/cmd/key.go` | New — `key export`, `key import`, `key fingerprint` subcommands |
| `cmd/msgvault/cmd/rotate_key.go` | New — Key rotation command (re-encrypt DEK with new KEK) |
| `Dockerfile` | SQLCipher build dependencies |

## Key Backup & Portability

The encryption key is tied to the OS keyring on the machine where the database was created. If you move the database to another machine, NAS, or restore from backup, you need the key. msgvault provides commands to export and import the key safely.

### Backup the Key

```bash
# Export key to a file (will prompt for confirmation)
msgvault key export --out ~/msgvault-key-backup.txt

# Export key to stdout (for piping to a secret manager)
msgvault key export --stdout

# Show key fingerprint (for verification, does NOT reveal the key)
msgvault key fingerprint
```

The exported key file contains the base64-encoded 256-bit key. **Treat this file like a password** — store it in a password manager, safe, or secondary keyring.

### Restore the Key on a New Machine

```bash
# Import key from a backup file
msgvault key import --from ~/msgvault-key-backup.txt

# Import key from stdin (e.g., from a password manager CLI)
op read "op://vault/msgvault/key" | msgvault key import --stdin

# Import and store in OS keyring (default)
msgvault key import --from ~/msgvault-key-backup.txt --provider keyring

# Import and use a different provider on the new machine
msgvault key import --from ~/msgvault-key-backup.txt --provider keyfile --keyfile-path /run/secrets/msgvault-key
```

### Moving a Database

```bash
# On the source machine:
msgvault key export --out /tmp/msgvault-key.txt
cp ~/.msgvault/msgvault.db /mnt/nas/msgvault.db

# On the destination machine:
msgvault key import --from /tmp/msgvault-key.txt
# Then securely delete the export:
shred -u /tmp/msgvault-key.txt  # Linux
# or: rm -P /tmp/msgvault-key.txt  # macOS
```

### Key Fingerprint Verification

To verify the same key is in use on both machines without exposing it:

```bash
# Source machine
msgvault key fingerprint
# Output: SHA-256: a1b2c3d4...

# Destination machine
msgvault key fingerprint
# Output: SHA-256: a1b2c3d4...  (should match)
```

### What Happens Without the Key

- Opening an encrypted database without the correct key **fails immediately** with a clear error (SQLCipher returns `SQLITE_NOTADB`)
- No data is corrupted — the database is intact, just inaccessible
- There is **no recovery** without the key — this is by design

## Migration Path

Existing unencrypted databases must be migrated:

1. `msgvault encrypt` — one-time command to encrypt an existing database
   - Creates new encrypted DB via SQLCipher `ATTACH` + copy
   - Encrypts existing attachment files in-place
   - Encrypts token files
   - Stores key in OS keyring (or configured provider)
   - **Prints a reminder to back up the key**
2. `msgvault decrypt` — reverse operation for export/migration
3. Encryption flag stored in DB metadata to prevent accidental unencrypted opens

## Network Impact

- **Vault provider**: Adds outbound HTTPS to Vault server address (must be added to container network whitelist)
- **All other providers**: No new network calls

## Test Plan

### Unit Tests

**`internal/encryption/provider_test.go`** — Key provider interface and implementations:

| Test | Description |
|---|---|
| `TestKeyfileProvider_GetKey` | Reads key from file, verifies 256-bit output |
| `TestKeyfileProvider_FileNotFound` | Returns clear error for missing key file |
| `TestKeyfileProvider_InvalidKey` | Rejects keys that aren't 256 bits |
| `TestEnvProvider_GetKey` | Reads base64 key from env var |
| `TestEnvProvider_Unset` | Returns error when env var not set |
| `TestPassphraseProvider_DeriveKey` | Argon2id derivation produces consistent key for same passphrase+salt |
| `TestExecProvider_GetKey` | Runs a command, reads key from stdout |
| `TestExecProvider_CommandFails` | Returns error on non-zero exit |

**`internal/encryption/keyring_test.go`** — OS keyring provider:

| Test | Description |
|---|---|
| `TestKeyringProvider_SetAndGet` | Round-trip store and retrieve (uses `keyring.MockInit()`) |
| `TestKeyringProvider_NotFound` | Returns clear error when no key stored |
| `TestKeyringProvider_MultipleDBs` | Independent keys for different DB paths |

**`internal/encryption/file_test.go`** — AES-256-GCM file encryption:

| Test | Description |
|---|---|
| `TestEncryptDecryptFile` | Round-trip: encrypt file, decrypt, compare to original |
| `TestEncryptDecryptFile_LargeFile` | 100MB+ file to verify streaming works |
| `TestDecryptFile_WrongKey` | Fails with authentication error, not garbage output |
| `TestDecryptFile_Tampered` | Modified ciphertext detected by GCM |
| `TestDecryptFile_Truncated` | Truncated file returns clear error |
| `TestEncryptFile_Idempotent` | Re-encrypting same plaintext produces different ciphertext (random nonce) |

**`internal/encryption/vault_test.go`** — HashiCorp Vault provider:

| Test | Description |
|---|---|
| `TestVaultKVProvider_GetKey` | Mock Vault HTTP server, verify KV v2 read |
| `TestVaultTransitProvider_WrapUnwrap` | Mock Transit encrypt/decrypt endpoints |
| `TestVaultProvider_AuthFailure` | Returns clear error on 403 |
| `TestVaultProvider_Unavailable` | Returns error on connection failure |

### Integration Tests

**`internal/store/store_encryption_test.go`** — SQLCipher integration:

| Test | Description |
|---|---|
| `TestEncryptedStore_Open` | Open encrypted DB, insert data, verify round-trip |
| `TestEncryptedStore_WrongKey` | Open with wrong key returns `SQLITE_NOTADB` |
| `TestEncryptedStore_FTS5` | Full-text search works on encrypted DB |
| `TestEncryptedStore_RawUnreadable` | Read DB file bytes directly, verify no plaintext (use `strings`-equivalent check) |
| `TestEncryptedStore_Migration` | Unencrypted DB → encrypted via `ATTACH` copy, verify all data intact |
| `TestEncryptedStore_Concurrent` | Multiple goroutines read/write encrypted DB |

**`cmd/msgvault/cmd/key_test.go`** — CLI key management:

| Test | Description |
|---|---|
| `TestKeyExportImport_Roundtrip` | Export key, import on fresh keyring, open DB |
| `TestKeyFingerprint_Matches` | Fingerprint is consistent for same key |
| `TestKeyExport_Stdout` | `--stdout` flag writes only key, no extra output |
| `TestKeyImport_Stdin` | Pipe key via stdin |
| `TestEncryptCommand_NewDB` | `msgvault encrypt` on unencrypted DB succeeds |
| `TestEncryptCommand_AlreadyEncrypted` | `msgvault encrypt` on encrypted DB is a no-op with warning |
| `TestDecryptCommand` | `msgvault decrypt` produces usable unencrypted DB |

### Performance Tests

**`internal/store/store_encryption_bench_test.go`**:

| Benchmark | Description |
|---|---|
| `BenchmarkInsert_Unencrypted` | Baseline insert throughput |
| `BenchmarkInsert_Encrypted` | Encrypted insert throughput (expect <15% delta) |
| `BenchmarkFTS5Search_Unencrypted` | Baseline FTS5 query |
| `BenchmarkFTS5Search_Encrypted` | Encrypted FTS5 query |
| `BenchmarkFileEncrypt_1MB` | AES-GCM file encryption throughput |
| `BenchmarkFileEncrypt_100MB` | Large file encryption throughput |

### Edge Case Tests

| Test | Description |
|---|---|
| `TestEncryptedStore_EmptyDB` | Encrypt an empty database |
| `TestEncryptedStore_CorruptHeader` | Corrupted file header returns error, not panic |
| `TestKeyRotation_MidOperation` | Key rotation while queries are in flight |
| `TestEncryptedAttachments_ContentAddressed` | Encrypted attachments still deduplicate by content hash |
| `TestProviderFallback` | Keyring unavailable → falls back to configured alternate |

## Documentation Updates

### README.md

Add to the **Features** list:
```markdown
- **Encryption at rest**: SQLCipher database encryption + AES-256-GCM file encryption with pluggable key providers (OS keychain, passphrase, keyfile, HashiCorp Vault)
```

Add new **Encryption** section after the existing Quick Start:
```markdown
## Encryption

msgvault supports encryption at rest for all stored data.

### Enable encryption on a new database
    msgvault init-db --encrypted

### Encrypt an existing database
    msgvault encrypt

### Key management
    msgvault key export --out ~/msgvault-key-backup.txt
    msgvault key import --from ~/msgvault-key-backup.txt
    msgvault key fingerprint

By default, the encryption key is stored in your OS keychain (macOS
Keychain, GNOME Keyring, or Windows Credential Manager). See the
[Encryption Guide](https://msgvault.io/guides/encryption/) for
alternative key providers including HashiCorp Vault.
```

### CLAUDE.md

Add to **Quick Commands**:
```markdown
./msgvault encrypt                              # Encrypt existing database
./msgvault decrypt                              # Decrypt for export/migration
./msgvault key export --out key.txt             # Backup encryption key
./msgvault key import --from key.txt            # Restore encryption key
./msgvault key fingerprint                      # Show key fingerprint
```

Add to **Configuration** section:
```markdown
[encryption]
enabled = true
provider = "keyring"  # keyring | passphrase | keyfile | env | vault | exec
```

Add to **Implementation Status > Completed** (when done):
```markdown
- **Encryption at rest**: SQLCipher DB encryption, AES-256-GCM file encryption, pluggable key providers
```

### docs/encryption.md (New)

Full encryption guide covering:
1. Overview — what is encrypted, threat model
2. Quick start — `init-db --encrypted` and `encrypt`
3. Key providers — configuration for each (keyring, passphrase, keyfile, env, vault, exec)
4. Key backup & portability — export, import, fingerprint, moving databases
5. Key rotation — `rotate-key` command
6. Container deployments — keyfile/env provider, Vault sidecar
7. Performance — expected overhead benchmarks
8. FAQ — wrong key behavior, recovery, multiple databases

### docs/api.md

Update API docs to note:
- Encrypted databases require key on server startup
- API server config supports `[encryption]` section

### TUI Keybindings / Help

Add to `?` help screen:
```
🔒  Database encrypted (SQLCipher)
```
Status indicator in TUI footer when encryption is active.

## Verification

1. All unit tests pass: `go test ./internal/encryption/...`
2. All integration tests pass: `go test ./internal/store/... -run Encrypt`
3. CLI tests pass: `go test ./cmd/msgvault/cmd/... -run Key`
4. Performance benchmarks within 15% overhead: `go test -bench BenchmarkEncrypt ./internal/store/...`
5. `strings msgvault.db` shows no plaintext email content
6. Startup fails gracefully with wrong key (clear error, no corruption)
7. README, CLAUDE.md, and docs/ are updated and accurate
