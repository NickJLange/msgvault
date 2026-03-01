# Encryption Performance Benchmarking

## Goal

Provide a data-driven comparison of msgvault's performance across different database drivers and encryption states.

## Benchmark Matrix

| Scenario | Driver | Encryption |
|---|---|---|
| Baseline | `github.com/mattn/go-sqlite3` | None |
| No-Encryption | `github.com/mutecomm/go-sqlcipher/v4` | None |
| Encrypted | `github.com/mutecomm/go-sqlcipher/v4` | SQLCipher Active |

## Metrics to Measure

1. **Insert Throughput**:
   - Time to insert 10,000 messages.
   - Impact of SQLCipher's WAL sync vs. standard SQLite WAL.
2. **Query Latency**:
   - Simple SELECT (single row by ID).
   - Complex JOIN (message list with participants).
   - Aggregate query (count by domain).
3. **FTS5 Search**:
   - Time to search 1,000,000 records using MATCH.
4. **Binary File I/O**:
   - Time to encrypt/decrypt 1MB, 10MB, and 100MB files via AES-256-GCM.

## Implementation Details

- **Test Harness**: Use Go's `testing.B` for all benchmarks.
- **Data Generator**: Create a realistic dataset of 100,000 messages with various attributes.
- **Reporting**: Output results in `benchcmp` compatible format.

## Verification

### Automated
- `make bench`: A target in the Makefile to run these specific benchmarks and output a summary report.

### Analysis
- Document any regressions found in the "No-Encryption" scenario (using the SQLCipher driver without a key) to ensure it's a suitable replacement for the stock SQLite driver.
- Confirm that "Encrypted" scenario overhead remains within the acceptable <15% threshold for most common operations.
