# iCloud-Backed Storage on macOS

## Goal

On macOS, default the msgvault data directory to a location backed up by iCloud Drive, so that email archives are automatically protected against data loss (disk failure, device loss, theft) without any user configuration.

## Background

The default data directory is `~/.msgvault/`, which is a dotfile in the home directory. On macOS, dotfiles are **not** synced by iCloud Drive. If a user's disk fails, their entire email archive is lost — potentially 20+ years of Gmail data that may have already been deleted from Gmail.

macOS iCloud Drive syncs `~/Library/Mobile Documents/` automatically. Applications can store data under `~/Library/Mobile Documents/iCloud~<bundle-id>/Documents/` (for apps with iCloud entitlements) or under a general-purpose path. For non-App-Store CLI tools, the conventional approach is to use `~/Library/Application Support/<app-name>/` which **is** backed up by Time Machine but **not** by iCloud, or to place data directly within `~/Library/Mobile Documents/com~apple~CloudDocs/` (the user's iCloud Drive root).

The most reliable approach for a CLI tool without Apple entitlements is to use a subdirectory within the user-visible iCloud Drive folder:

```
~/Library/Mobile Documents/com~apple~CloudDocs/msgvault/
```

This appears in Finder as `iCloud Drive > msgvault` and is automatically synced to iCloud.

## Performance Considerations

| Aspect | Impact | Mitigation |
|---|---|---|
| **SQLite WAL sync** | iCloud may interfere with WAL journaling if it syncs a partial write. SQLite's WAL is crash-safe locally, but iCloud's upload timing is not coordinated with SQLite transactions. | Use `.nosync` extension on WAL/SHM files (see below) |
| **Large file uploads** | Initial sync of a multi-GB database will saturate upload bandwidth. iCloud throttles uploads and may take hours/days for large archives. | Expected behavior — no mitigation needed. Sync is background. |
| **Write latency** | iCloud Drive has **no measurable write latency overhead** — writes go to the local filesystem first, iCloud syncs asynchronously in the background. | None needed |
| **Conflict resolution** | If msgvault runs on two Macs with the same iCloud account, iCloud may create conflict copies of the SQLite database. | Document single-writer requirement. Detect and warn on conflict files. |
| **Disk space** | iCloud may evict (offload) large files to free local space. An evicted `.db` file would fail to open. | Use `com.apple.metadata:com_apple_backup_excludeItem` or `setResourceValue(.excludedFromBackupKey)` to **pin** the database locally while still syncing. Alternative: place a `.nosync` on the DB and only sync exports. |

### SQLite + iCloud Safety

SQLite WAL and SHM files must **not** be synced by iCloud — they are transient and machine-specific. iCloud respects the `.nosync` extension convention:

```
msgvault.db          → synced by iCloud ✓
msgvault.db-wal      → must NOT sync (transient)
msgvault.db-shm      → must NOT sync (transient)
```

**Strategy**: Create `.nosync` sentinel files or use `xattr` to exclude WAL/SHM:

```bash
# iCloud respects these sentinel files
touch ~/Library/Mobile Documents/com~apple~CloudDocs/msgvault/msgvault.db-wal.nosync
touch ~/Library/Mobile Documents/com~apple~CloudDocs/msgvault/msgvault.db-shm.nosync
```

Alternatively, use the `com.apple.fileprovider.ignore` extended attribute on WAL/SHM files programmatically.

**Recommended approach**: Open SQLite with `PRAGMA journal_mode=WAL` as usual, but create companion `.nosync` marker files for `-wal` and `-shm` on startup. This is how apps like DEVONthink and Obsidian handle iCloud + SQLite safely.

### Eviction Risk

iCloud may "evict" (offload) large files to free local disk space, replacing them with a placeholder. An evicted `msgvault.db` would fail to open with a confusing error.

**Mitigation options** (in order of preference):

1. **Pin the database file**: Call `URL.setResourceValue(true, forKey: .isExcludedFromEvictionKey)` — requires calling a small Swift helper or using `xattr`. This keeps the file synced but prevents eviction.
2. **Detect eviction**: On open failure, check if the file has the `com.apple.ubiquity.isevicted` xattr. If so, show a clear error: "Your database has been offloaded to iCloud. Open Finder and click the download icon next to msgvault.db to restore it."
3. **Exclude DB from iCloud, sync only exports**: Place `.nosync` on the DB itself and only sync exported backups. Loses automatic protection.

## Proposed Design

### Detection Logic

```go
func isICloudAvailable() bool {
    if runtime.GOOS != "darwin" {
        return false
    }
    icloudRoot := filepath.Join(os.Getenv("HOME"),
        "Library", "Mobile Documents", "com~apple~CloudDocs")
    info, err := os.Stat(icloudRoot)
    return err == nil && info.IsDir()
}
```

### Default Path Selection (macOS only)

On `init-db` (first run), if no `MSGVAULT_HOME` is set and no existing `~/.msgvault/` exists:

1. Check if iCloud Drive is available
2. If yes, default to `~/Library/Mobile Documents/com~apple~CloudDocs/msgvault/`
3. If no, fall back to `~/.msgvault/`
4. Print a message explaining the choice

If `~/.msgvault/` already exists (existing user), do **not** move it automatically. Offer a migration command instead.

### Migration Command

```bash
# Move existing data to iCloud-backed location
msgvault migrate-storage --to icloud

# Move back to traditional location
msgvault migrate-storage --to local

# Show current storage location and iCloud status
msgvault storage-info
```

`migrate-storage --to icloud`:
1. Verify iCloud Drive is available
2. Close any open database connections
3. Move `~/.msgvault/` contents to iCloud path
4. Create a symlink `~/.msgvault/ → iCloud path` for compatibility
5. Create `.nosync` markers for WAL/SHM files
6. Update `config.toml` with new path
7. Verify database opens correctly at new location

### iCloud Sync Safety on DB Open

On every database open (macOS only):

```go
func ensureICloudSafe(dbPath string) error {
    if runtime.GOOS != "darwin" {
        return nil
    }
    // Only act if the DB is within iCloud Drive
    if !isInICloudDrive(dbPath) {
        return nil
    }
    // Create .nosync markers for WAL/SHM
    for _, suffix := range []string{"-wal", "-shm"} {
        nosync := dbPath + suffix + ".nosync"
        if _, err := os.Stat(nosync); os.IsNotExist(err) {
            os.WriteFile(nosync, nil, 0644)
        }
    }
    // Check for eviction
    if isEvicted(dbPath) {
        return fmt.Errorf("database file has been offloaded to iCloud; " +
            "open Finder → iCloud Drive → msgvault and click the download icon to restore it")
    }
    // Check for conflict copies
    conflicts, _ := filepath.Glob(dbPath[:len(dbPath)-3] + " *.db")
    if len(conflicts) > 0 {
        slog.Warn("iCloud conflict copies detected — ensure only one machine writes to this database",
            "conflicts", conflicts)
    }
    return nil
}
```

### Config Changes

```toml
[data]
# Automatically set on macOS with iCloud; can be overridden
# data_dir = "~/Library/Mobile Documents/com~apple~CloudDocs/msgvault"

[data.icloud]
enabled = true            # Whether iCloud storage is active (macOS only)
pin_database = true       # Prevent iCloud from evicting the database file
```

## Proposed Changes

| File | Change |
|---|---|
| `internal/config/config.go` | Add `ICloudConfig` struct, update `DefaultHome()` to check iCloud on macOS |
| `internal/config/icloud_darwin.go` | New — macOS-specific iCloud detection, eviction check, `.nosync` management |
| `internal/config/icloud_other.go` | New — no-op stubs for non-macOS platforms |
| `internal/store/store.go` | Call `ensureICloudSafe()` on database open |
| `cmd/msgvault/cmd/migrate_storage.go` | New — `migrate-storage` command (--to icloud/local) |
| `cmd/msgvault/cmd/storage_info.go` | New — `storage-info` command showing location + iCloud status |
| `cmd/msgvault/cmd/init_db.go` | Update to select iCloud path on macOS first run |

## Verification

1. On macOS with iCloud: `msgvault init-db` creates database under iCloud Drive path
2. Database appears in `Finder → iCloud Drive → msgvault`
3. `.nosync` markers exist for `-wal` and `-shm` files
4. `msgvault storage-info` shows iCloud status, sync state, eviction status
5. `msgvault migrate-storage --to icloud` moves existing `~/.msgvault/` successfully
6. `msgvault migrate-storage --to local` moves back, removes symlink
7. On Linux/Windows: iCloud logic is completely inactive (build tags)
8. Evicted database file produces clear error message, not a crash
9. Conflict copies produce a warning log, not a crash
10. Performance: write throughput benchmarks show no regression (iCloud syncs asynchronously)
