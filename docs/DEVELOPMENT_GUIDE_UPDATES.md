# Development Guide Updates for Encryption Support

This document describes changes needed to the msgvault.io/development page to reflect encryption-at-rest support.

## Required Changes to https://www.msgvault.io/development/

### 1. Update Dependencies Table

Add these entries to the existing Dependencies section:

```
| Library | Purpose |
| `mutecomm/go-sqlcipher/v4` | SQLite encryption (SQLCipher) for database-at-rest encryption |
| `duckdb/duckdb-go/v2` | DuckDB driver for Parquet analytics with Modular Encryption support |
```

### 2. Add Encryption Section to Code Conventions

Add after the existing Code Conventions:

```markdown
**Encryption** : 
- Database encryption: SQLCipher (mutecomm/go-sqlcipher/v4) for at-rest encryption
- Parquet encryption: DuckDB Modular Encryption with base64-encoded 32-byte keys
- Key management: Pluggable providers (keyring, passphrase, keyfile, env, exec)
- File encryption: AES-256-GCM for attachments and tokens
- All DB operations must use store.Open() or store.OpenEncrypted() through the Store struct
```

### 3. Build Flags Note

Add to Lint & Format section:

```markdown
**Note on Build Flags** : 
- FTS5 support requires CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" during build
- This is configured in Makefile, Dockerfile, and flake.nix
- Encryption support requires mutecomm/go-sqlcipher with SQLCipher 4.0+
```

## Local CLAUDE.md

The CLAUDE.md in this repository already documents encryption conventions and should not need updates.
