# Embedding-Based Semantic Search

## Goal

Add semantic search to msgvault so users can find emails by meaning rather than exact keyword matches. For example, "messages about rescheduling meetings" should find emails that say "can we move our call to Thursday" even without the word "reschedule".

## Current State

Search is keyword-based:
- **FTS5**: Full-text search over subject + body (`messages_fts` virtual table)
- **SearchFast**: Metadata-only search over subject, sender (DuckDB/Parquet)
- **Query parser**: Gmail-style operators (`from:`, `to:`, `has:attachment`, etc.)

No embeddings, no vector storage, no semantic matching.

## Proposed Design

### Embedding Model

Use a local embedding model to keep the offline-first promise. Options:

| Model | Dimensions | Speed | Quality | Size |
|---|---|---|---|---|
| `nomic-embed-text` (via Ollama) | 768 | Fast | Good | ~270MB |
| `all-MiniLM-L6-v2` (native Go) | 384 | Very fast | Good | ~80MB |
| `mxbai-embed-large` (via Ollama) | 1024 | Moderate | Best | ~670MB |

**Recommendation**: Support Ollama-hosted models (leverages existing `ChatConfig.Server`), with the embedding model configurable. This aligns with the existing Ollama config and avoids bundling model weights into the binary.

### Storage

Store embeddings in SQLite alongside existing data:

```sql
CREATE TABLE message_embeddings (
    message_id INTEGER PRIMARY KEY REFERENCES messages(id),
    model      TEXT NOT NULL,          -- model used (for invalidation on model change)
    embedding  BLOB NOT NULL,          -- float32 vector, stored as raw bytes
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**Why SQLite over a vector DB**: Single-file simplicity matches msgvault's architecture. For archive sizes (100k–1M messages), brute-force cosine similarity over BLOB vectors is fast enough (~50ms for 500k vectors). If needed later, add an IVF index or switch to sqlite-vec extension.

### What Gets Embedded

Concatenate and embed per message:
```
Subject: <subject>
From: <sender>
<body text, truncated to model's context window>
```

### Architecture

```
internal/
├── embedding/
│   ├── embedder.go       # Embedder interface + Ollama implementation
│   ├── store.go          # SQLite embedding storage (read/write/batch)
│   └── indexer.go        # Background indexing orchestrator
```

### Integration Points

1. **`search.Query`** — Add `Semantic bool` field or detect non-operator free-text queries
2. **`query.Engine`** — Add `SearchSemantic(ctx, queryText, filter, limit, offset)` method
3. **`config.ChatConfig`** — Extend with `EmbeddingModel` field (reuse `Server` for Ollama URL)
4. **CLI** — `msgvault build-embeddings [--model nomic-embed-text]` command
5. **TUI** — Semantic search toggle or automatic fallback when FTS5 returns few results

### Search Flow

```
User query "rescheduling meetings"
    │
    ├─► FTS5 keyword search (existing, fast)
    │       └─► Returns keyword matches
    │
    └─► Semantic search (new)
            ├─► Embed query text via Ollama
            ├─► Cosine similarity against message_embeddings
            ├─► Return top-K by similarity score
            └─► Merge/rank with FTS5 results
```

### Indexing Strategy

- **Parallel embedding**: Use a worker pool (default: 4 workers, configurable) sending concurrent requests to Ollama. Ollama handles its own GPU scheduling, so multiple in-flight requests keep the pipeline saturated. Batch messages into groups for efficient throughput.
- **Initial build**: Parallel batch process all messages. With 4 workers, expect ~400 messages/second via Ollama (GPU-dependent).
- **Incremental**: After each sync, embed new messages only (check `message_embeddings` for missing IDs)
- **Auto-build**: Similar to Parquet cache — detect unembedded messages on TUI launch, offer to build
- **Progress**: Show progress bar during embedding build (similar to `build-cache`)
- **Resumable**: Track progress so interrupted builds resume where they left off

## Network Impact

Ollama runs locally (`localhost:11434` by default), so this adds **no new remote network calls**. For containerized deployments, the Ollama server would be a sidecar container or accessed via the host network.

If a cloud embedding API is desired in the future, it would be a separate opt-in provider behind the same `Embedder` interface.

## Configuration

```toml
[chat]
server = "http://localhost:11434"
model = "gpt-oss-128k"             # chat model (existing)
embedding_model = "nomic-embed-text" # embedding model (new)
embedding_workers = 4                # parallel embedding workers (new)
max_results = 20
```

## Proposed Changes

| File | Change |
|---|---|
| `internal/embedding/embedder.go` | New — `Embedder` interface, Ollama HTTP client |
| `internal/embedding/store.go` | New — SQLite read/write for embedding vectors |
| `internal/embedding/indexer.go` | New — Batch + incremental indexing orchestrator |
| `internal/store/schema.sql` | Add `message_embeddings` table |
| `internal/query/engine.go` | Add `SearchSemantic` to `Engine` interface |
| `internal/query/sqlite.go` | Implement semantic search (load vectors, cosine sim) |
| `internal/search/parser.go` | Detect semantic vs keyword queries |
| `internal/config/config.go` | Add `EmbeddingModel`, `EmbeddingWorkers` to `ChatConfig` |
| `cmd/msgvault/cmd/build_embeddings.go` | New — CLI command for embedding build |
| `internal/tui/model.go` | Integrate semantic search results |

## Verification

1. `msgvault build-embeddings` completes without error, populates `message_embeddings`
2. Semantic query "travel plans" returns emails about flights/hotels even without those exact words
3. Embedding build is incremental — re-running only processes new messages
4. Changing `embedding_model` in config triggers re-embedding (model mismatch detection)
5. Search works without embeddings (graceful fallback to FTS5-only)
6. No new remote network calls (Ollama is localhost)
