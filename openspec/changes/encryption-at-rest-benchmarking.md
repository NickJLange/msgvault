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

## Initial Benchmark Results (Feb 28, 2026)

Tested on Apple M2 Max, 64GB RAM.

| Metric | Driver | Encryption | Result |
|---|---|---|---|
| Insert (10k msgs) | `go-sqlite3` | None | 164 µs/msg |
| Insert (10k msgs) | `go-sqlcipher` | Active | 833 µs/msg (~5x) |
| Query (Stats) | Both | Both | ~30 µs/op (<1% delta) |
| FTS5 Search | Both | Both | ~370 µs/op (<1% delta) |
| File Encrypt (1MB) | AES-GCM | Active | 384 µs (2.7 GB/s) |
| File Decrypt (1MB) | AES-GCM | Active | 234 µs (4.4 GB/s) |

### Analysis
- **Read Operations**: Encryption overhead is effectively zero for reads and FTS5 searches.
- **Write Operations**: The 5x insert overhead is consistent with SQLCipher's security model (additional HMACs and page synchronization).
- **File I/O**: AES-GCM is hardware-accelerated and poses no bottleneck for attachments or tokens.
