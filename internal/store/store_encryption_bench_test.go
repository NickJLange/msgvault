package store_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/wesm/msgvault/internal/encryption"
	"github.com/wesm/msgvault/internal/store"
)

// setupBenchStore creates a store for benchmarking. If key is non-nil, opens encrypted.
func setupBenchStore(b *testing.B, key []byte) *store.Store {
	b.Helper()
	dbPath := filepath.Join(b.TempDir(), "bench.db")
	var st *store.Store
	var err error
	if key != nil {
		st, err = store.OpenEncrypted(dbPath, key)
	} else {
		st, err = store.Open(dbPath)
	}
	if err != nil {
		b.Fatalf("open store: %v", err)
	}
	if err := st.InitSchema(); err != nil {
		b.Fatalf("init schema: %v", err)
	}
	b.Cleanup(func() { st.Close() })
	return st
}

// seedMessages inserts n messages into the store for read benchmarks.
func seedMessages(b *testing.B, st *store.Store, n int) int64 {
	b.Helper()
	source, err := st.GetOrCreateSource("gmail", "bench@example.com")
	if err != nil {
		b.Fatalf("GetOrCreateSource: %v", err)
	}

	convID, err := st.EnsureConversation(source.ID, "conv-1", "Bench Thread")
	if err != nil {
		b.Fatalf("EnsureConversation: %v", err)
	}

	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		msg := &store.Message{
			ConversationID:  convID,
			SourceID:        source.ID,
			SourceMessageID: fmt.Sprintf("msg-%d", i),
			MessageType:     "email",
			SentAt:          sql.NullTime{Time: baseTime.Add(time.Duration(i) * time.Hour), Valid: true},
			Subject:         sql.NullString{String: fmt.Sprintf("Benchmark subject %d with some content", i), Valid: true},
			Snippet:         sql.NullString{String: fmt.Sprintf("This is the snippet for message %d, containing enough text to be realistic.", i), Valid: true},
			SizeEstimate:    int64(1000 + i%5000),
		}
		if _, err := st.UpsertMessage(msg); err != nil {
			b.Fatalf("UpsertMessage: %v", err)
		}
	}
	return source.ID
}

func BenchmarkInsert_Unencrypted(b *testing.B) {
	st := setupBenchStore(b, nil)
	source, _ := st.GetOrCreateSource("gmail", "bench@example.com")
	convID, _ := st.EnsureConversation(source.ID, "conv-1", "Bench Thread")
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &store.Message{
			ConversationID:  convID,
			SourceID:        source.ID,
			SourceMessageID: fmt.Sprintf("msg-%d", i),
			MessageType:     "email",
			SentAt:          sql.NullTime{Time: baseTime.Add(time.Duration(i) * time.Hour), Valid: true},
			Subject:         sql.NullString{String: fmt.Sprintf("Subject %d", i), Valid: true},
			Snippet:         sql.NullString{String: fmt.Sprintf("Snippet for message %d with realistic content length.", i), Valid: true},
			SizeEstimate:    2500,
		}
		if _, err := st.UpsertMessage(msg); err != nil {
			b.Fatalf("UpsertMessage: %v", err)
		}
	}
}

func BenchmarkInsert_Encrypted(b *testing.B) {
	key, _ := encryption.GenerateKey()
	st := setupBenchStore(b, key)
	source, _ := st.GetOrCreateSource("gmail", "bench@example.com")
	convID, _ := st.EnsureConversation(source.ID, "conv-1", "Bench Thread")
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := &store.Message{
			ConversationID:  convID,
			SourceID:        source.ID,
			SourceMessageID: fmt.Sprintf("msg-%d", i),
			MessageType:     "email",
			SentAt:          sql.NullTime{Time: baseTime.Add(time.Duration(i) * time.Hour), Valid: true},
			Subject:         sql.NullString{String: fmt.Sprintf("Subject %d", i), Valid: true},
			Snippet:         sql.NullString{String: fmt.Sprintf("Snippet for message %d with realistic content length.", i), Valid: true},
			SizeEstimate:    2500,
		}
		if _, err := st.UpsertMessage(msg); err != nil {
			b.Fatalf("UpsertMessage: %v", err)
		}
	}
}

func BenchmarkQueryStats_Unencrypted(b *testing.B) {
	st := setupBenchStore(b, nil)
	seedMessages(b, st, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.GetStats(); err != nil {
			b.Fatalf("GetStats: %v", err)
		}
	}
}

func BenchmarkQueryStats_Encrypted(b *testing.B) {
	key, _ := encryption.GenerateKey()
	st := setupBenchStore(b, key)
	seedMessages(b, st, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.GetStats(); err != nil {
			b.Fatalf("GetStats: %v", err)
		}
	}
}

func BenchmarkFTS5Search_Unencrypted(b *testing.B) {
	st := setupBenchStore(b, nil)
	if !st.FTS5Available() {
		b.Skip("FTS5 not available")
	}
	seedMessages(b, st, 1000)

	if _, err := st.BackfillFTS(nil); err != nil {
		b.Fatalf("BackfillFTS: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := st.DB().Query("SELECT rowid FROM messages_fts WHERE messages_fts MATCH 'benchmark'")
		if err != nil {
			b.Fatalf("FTS query: %v", err)
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				b.Fatalf("rows.Scan: %v", err)
			}
		}
		rows.Close()
	}
}

func BenchmarkFTS5Search_Encrypted(b *testing.B) {
	key, err := encryption.GenerateKey()
	if err != nil {
		b.Fatalf("GenerateKey: %v", err)
	}
	st := setupBenchStore(b, key)
	if !st.FTS5Available() {
		b.Skip("FTS5 not available")
	}
	seedMessages(b, st, 1000)

	if _, err := st.BackfillFTS(nil); err != nil {
		b.Fatalf("BackfillFTS: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := st.DB().Query("SELECT rowid FROM messages_fts WHERE messages_fts MATCH 'benchmark'")
		if err != nil {
			b.Fatalf("FTS query: %v", err)
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				b.Fatalf("rows.Scan: %v", err)
			}
		}
		rows.Close()
	}
}

func BenchmarkFileEncrypt_1MB(b *testing.B) {
	key, err := encryption.GenerateKey()
	if err != nil {
		b.Fatalf("GenerateKey: %v", err)
	}
	data := make([]byte, 1<<20) // 1 MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := encryption.EncryptBytes(key, data); err != nil {
			b.Fatalf("EncryptBytes: %v", err)
		}
	}
}

func BenchmarkFileDecrypt_1MB(b *testing.B) {
	key, _ := encryption.GenerateKey()
	data := make([]byte, 1<<20) // 1 MB
	for i := range data {
		data[i] = byte(i % 256)
	}
	encrypted, _ := encryption.EncryptBytes(key, data)

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := encryption.DecryptBytes(key, encrypted); err != nil {
			b.Fatalf("DecryptBytes: %v", err)
		}
	}
}
