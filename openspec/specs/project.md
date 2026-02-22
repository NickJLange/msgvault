# msgvault — Project Spec

## Overview

msgvault is an offline Gmail archive tool that exports and stores email data locally with full-text search capabilities. Single-binary Go application.

## Network Access Audit

Conducted 2026-02-22. All outbound network calls from the application:

### 1. Google Gmail API (Core — Required)

| Endpoint | File | Purpose |
|---|---|---|
| `https://gmail.googleapis.com/gmail/v1/*` | `internal/gmail/client.go` | Gmail sync (list/get messages), deletion execution |
| Google OAuth2 token endpoints | `internal/oauth/oauth.go` | Token acquisition, refresh, device-code flow |
| OAuth scopes: `gmail.readonly`, `gmail.modify`, `mail.google.com` | `internal/oauth/oauth.go` | Permission scopes |

**Protocol**: HTTPS only. Uses `golang.org/x/oauth2/google` for auth, custom HTTP client with rate limiting for API calls.

### 2. GitHub API (Auto-Update)

| Endpoint | File | Purpose |
|---|---|---|
| `https://api.github.com/repos/wesm/msgvault/releases/latest` | `internal/update/update.go` | Check for new releases |
| GitHub release asset download URLs | `internal/update/update.go` | Download binaries + checksums |

**Trigger**: Automatic on CLI startup (cached for 1 hour). Can be disabled.

### 3. Remote msgvault Server (User-Configured, Opt-In)

| Endpoint | File | Purpose |
|---|---|---|
| User-specified URL (e.g. `https://nas:8080`) | `internal/remote/store.go` | Sync data to remote msgvault instance |
| Same user-specified URL | `cmd/msgvault/cmd/export_token.go` | Upload OAuth tokens to remote instance |

**Trigger**: Only when user explicitly configures `[remote]` in config.toml or uses `export-token` command. HTTPS enforced by default; HTTP requires `--allow-insecure`.

### 4. Ollama / LLM (Configured, Not Active)

| Endpoint | File | Purpose |
|---|---|---|
| `http://localhost:11434` (default) | `internal/config/config.go` | Chat/LLM config exists but no outbound calls implemented |

**Status**: Config struct exists; no code makes outbound HTTP calls to this endpoint.

### 5. Local-Only Listeners (Inbound, Not Outbound)

| Listener | File | Purpose |
|---|---|---|
| `127.0.0.1:8080` (configurable) | `internal/api/server.go` | HTTP API server for remote access |
| Localhost callback (ephemeral port) | `internal/oauth/oauth.go` | OAuth browser redirect receiver |
| stdio | `internal/mcp/server.go` | MCP server for LLM tool integration |

### Network Whitelist Summary

For a restricted container, these domains must be reachable:

| Domain | Port | Purpose |
|---|---|---|
| `gmail.googleapis.com` | 443 | Gmail API |
| `oauth2.googleapis.com` | 443 | OAuth2 token exchange |
| `accounts.google.com` | 443 | OAuth2 authorization |
| `api.github.com` | 443 | Update checks (optional) |
| `github.com` | 443 | Binary downloads (optional) |
| `objects.githubusercontent.com` | 443 | GitHub release assets (optional) |

## Search Architecture

msgvault does **not** use embeddings or vector search. Search is implemented via:

- **SQLite FTS5**: Full-text search over message subjects and bodies (`messages_fts` virtual table)
- **DuckDB over Parquet**: Fast aggregate analytics for TUI (senders, domains, labels, time series)
- **Gmail-style query parser**: `from:`, `to:`, `has:attachment`, `after:`, `before:` etc. (`internal/search/parser.go`)

## Data Storage

All data stored under `~/.msgvault/` (overridable via `MSGVAULT_HOME`):

| Path | Format | Purpose |
|---|---|---|
| `msgvault.db` | SQLite | System of record — messages, metadata, FTS5 index |
| `analytics/messages/year=*/` | Parquet (partitioned) | Denormalized analytics cache for TUI |
| `analytics/_last_sync.json` | JSON | Incremental cache sync state |
| `attachments/` | Content-addressed files | Deduplicated attachment storage |
| `tokens/` | JSON | OAuth tokens per account |
| `config.toml` | TOML | Application configuration |
