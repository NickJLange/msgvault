package sync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wesm/msgvault/internal/gmail"
	"github.com/wesm/msgvault/internal/store"
)

type TestEnv struct {
	Store   *store.Store
	Mock    *gmail.MockAPI
	Syncer  *Syncer
	TmpDir  string
	Context context.Context
}

func NewTestEnv(t *testing.T) (*TestEnv, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "msgvault-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("open store: %v", err)
	}

	if err := st.InitSchema(); err != nil {
		st.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("init schema: %v", err)
	}

	mock := gmail.NewMockAPI()
	// Default profile setup
	mock.Profile = &gmail.Profile{
		EmailAddress:  "test@example.com",
		MessagesTotal: 0,
		HistoryID:     1000,
	}

	env := &TestEnv{
		Store:   st,
		Mock:    mock,
		TmpDir:  tmpDir,
		Context: context.Background(),
	}

	// Helper to create syncer easily
	env.Syncer = New(mock, st, nil)

	cleanup := func() {
		st.Close()
		os.RemoveAll(tmpDir)
	}
	return env, cleanup
}

func (e *TestEnv) SetupSource(t *testing.T, historyID string) {
	t.Helper()
	source, err := e.Store.GetOrCreateSource("gmail", e.Mock.Profile.EmailAddress)
	if err != nil {
		t.Fatalf("GetOrCreateSource: %v", err)
	}
	if err := e.Store.UpdateSourceSyncCursor(source.ID, historyID); err != nil {
		t.Fatalf("UpdateSourceSyncCursor: %v", err)
	}
}
