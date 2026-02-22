# Container with Restricted Network Access

## Goal

Run msgvault in a Docker container with network access restricted to only the domains it needs, minimizing attack surface for an application handling sensitive email data.

## Background

Network audit (see `openspec/specs/project.md`) identified that msgvault only requires access to:
- Google Gmail API + OAuth2 endpoints (core functionality)
- GitHub API (optional — auto-update checks)

All other network features (remote server sync, Ollama) are user-opt-in and not needed for the primary archive use case.

## Proposed Changes

### 1. Dockerfile enhancements (`Dockerfile`)

- Use multi-stage build: builder stage (Go + DuckDB) → minimal runtime (distroless or alpine)
- Run as non-root user
- Mount volume for `~/.msgvault/` data persistence
- Expose port 8080 only if API server is needed

### 2. Docker Compose with network policy (`docker-compose.yml`)

- Define a custom network with restricted egress
- Use DNS-based allowlist via a network proxy sidecar (e.g., squid or envoy) or iptables rules in an init container
- Allowed domains:
  - `gmail.googleapis.com`
  - `oauth2.googleapis.com`
  - `accounts.google.com`
  - `api.github.com` (optional, can be removed if updates disabled)

### 3. Network restriction approach options

**Option A — Squid proxy sidecar**:
- Run squid as a sidecar with domain allowlist
- Set `HTTP_PROXY`/`HTTPS_PROXY` in msgvault container
- Pros: DNS-based filtering, logs, easy to audit
- Cons: Extra container, slight complexity

**Option B — iptables init container**:
- Resolve allowed domains at startup, install iptables rules
- Pros: No sidecar, lightweight
- Cons: DNS can change, requires `NET_ADMIN` capability

**Option C — Docker network + external firewall**:
- Use Docker's built-in network isolation + host firewall rules
- Pros: Simplest setup
- Cons: Less granular, host-dependent

### 4. OAuth flow consideration

The browser-based OAuth flow requires the user's browser to access Google's auth page, but the *container* only needs to receive the callback and exchange the token. For headless/container deployments:
- Use `--headless` device-code flow (no browser needed in container)
- Or use `export-token` to upload a token from a machine with a browser

### 5. Configuration for container mode

Add to `config.toml`:
```toml
[sync]
# Disable auto-update checks in container (managed by image rebuild)
disable_update_check = true
```

## Verification

1. Build container: `docker build -t msgvault .`
2. Run with restricted network and verify:
   - `msgvault sync-full` succeeds (Gmail API reachable)
   - `msgvault tui` works (local-only, no network needed)
   - Arbitrary outbound connections fail (e.g., `curl https://example.com` blocked)
3. Test OAuth device-code flow from within container
4. Verify data persists across container restarts via mounted volume
