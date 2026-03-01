# Encryption at Rest — PR Review Issues

Tracked issues from PR #1 code review. All issues are on [NickJLange/msgvault](https://github.com/NickJLange/msgvault/issues).

## Critical (2)

| # | Title | File(s) |
|---|-------|---------|
| [#2](https://github.com/NickJLange/msgvault/issues/2) | encrypt: keyring error handling treats all errors as 'no key' | `cmd/msgvault/cmd/encrypt.go` L46 |
| [#3](https://github.com/NickJLange/msgvault/issues/3) | rotate_key: key rotation is not transaction-safe | `cmd/msgvault/cmd/rotate_key.go` |

## Major (10)

| # | Title | File(s) |
|---|-------|---------|
| [#4](https://github.com/NickJLange/msgvault/issues/4) | key import: does not update provider in config when already enabled | `cmd/msgvault/cmd/key.go` L208 |
| [#5](https://github.com/NickJLange/msgvault/issues/5) | encrypt/decrypt: filepath.Walk swallows directory traversal errors | `cmd/msgvault/cmd/encrypt.go` L107, L214 |
| [#6](https://github.com/NickJLange/msgvault/issues/6) | encrypt: ATTACH SQL does not escape single quotes in dbPath | `cmd/msgvault/cmd/encrypt.go` L270 |
| [#7](https://github.com/NickJLange/msgvault/issues/7) | rotate_key: keyfile write is not atomic | `cmd/msgvault/cmd/rotate_key.go` L125 |
| [#8](https://github.com/NickJLange/msgvault/issues/8) | store: OpenEncrypted stores caller-owned key without defensive copy | `internal/store/store.go` L119 |
| [#9](https://github.com/NickJLange/msgvault/issues/9) | duckdb: silently truncates encryption keys longer than 32 bytes | `internal/query/duckdb.go` L137 |
| [#10](https://github.com/NickJLange/msgvault/issues/10) | store_resolver: uses context.Background() instead of caller context | `cmd/msgvault/cmd/store_resolver.go` L76 |
| [#11](https://github.com/NickJLange/msgvault/issues/11) | build_cache: getEncryptionKey should validate key length | `cmd/msgvault/cmd/build_cache.go` |
| [#12](https://github.com/NickJLange/msgvault/issues/12) | flake.nix: invalid cflags attribute, FTS5 silently missing | `flake.nix` L36 |
| [#13](https://github.com/NickJLange/msgvault/issues/13) | config: missing expandPath for keyfile path | `internal/config/config.go` |

## Minor (6)

| # | Title | File(s) |
|---|-------|---------|
| [#14](https://github.com/NickJLange/msgvault/issues/14) | tui: cfg.Encryption.Enabled vs s.IsEncrypted() | `cmd/msgvault/cmd/tui.go` L101, L127 |
| [#15](https://github.com/NickJLange/msgvault/issues/15) | exec provider: hardcoded sh -c breaks on Windows | `internal/encryption/exec.go` L30 |
| [#16](https://github.com/NickJLange/msgvault/issues/16) | docs: overly strict SQLCipher version pin | `DEVELOPMENT_GUIDE_UPDATES.md` L38 |
| [#17](https://github.com/NickJLange/msgvault/issues/17) | benchmarks: discarded errors | bench tests L69, L92, L219, L224 |
| [#18](https://github.com/NickJLange/msgvault/issues/18) | keyring_test: discarded GenerateKey error | `keyring_test.go` L108 |
| [#19](https://github.com/NickJLange/msgvault/issues/19) | file.go: os.Rename not atomic on Windows | `internal/encryption/file.go` L132 |

## Suggested Fix Order

1. **Critical first**: #2 and #3 (data loss risks)
2. **Major bugs**: #4–#11 (incorrect behavior)
3. **Build/config**: #12, #13 (affects usability)
4. **Minor/nitpick**: #14–#19 (low risk, easy fixes)

## Branch Info

- PR: https://github.com/NickJLange/msgvault/pull/1
- Branch: `feature/encryption-at-rest`
- Origin: `git@github.com:NickJLange/msgvault.git`
- Backup tag: `backup-before-rebase` at `791b58a`
