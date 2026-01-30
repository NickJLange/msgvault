package tui

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/wesm/msgvault/internal/deletion"
	"github.com/wesm/msgvault/internal/query"
)

func testActionController(t *testing.T, engine *mockEngine) (*ActionController, string) {
	t.Helper()
	dir := t.TempDir()
	mgr, err := deletion.NewManager(filepath.Join(dir, "deletions"))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return NewActionController(engine, dir, mgr), dir
}

func newTestController(t *testing.T, gmailIDs ...string) *ActionController {
	t.Helper()
	ctrl, _ := testActionController(t, &mockEngine{gmailIDs: gmailIDs})
	return ctrl
}

type stageArgs struct {
	aggregates map[string]bool
	selection  map[int64]bool
	view       query.ViewType
	accountID  *int64
	accounts   []query.AccountInfo
	messages   []query.MessageSummary
}

func stageForDeletion(t *testing.T, ctrl *ActionController, args stageArgs) *deletion.Manifest {
	t.Helper()
	view := args.view
	if view == 0 {
		view = query.ViewSenders
	}
	manifest, err := ctrl.StageForDeletion(
		args.aggregates, args.selection, view, args.accountID, args.accounts,
		view, "", query.TimeYear, args.messages,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return manifest
}

func stringSet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}
	return m
}

func idSet(ids ...int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func msgSummaries(sourceIDs ...string) []query.MessageSummary {
	out := make([]query.MessageSummary, len(sourceIDs))
	for i, sid := range sourceIDs {
		out[i] = query.MessageSummary{ID: int64(i + 1), SourceMessageID: sid}
	}
	return out
}

func assertSingleFilter(t *testing.T, got []string, want string, label string) {
	t.Helper()
	if len(got) != 1 || got[0] != want {
		t.Errorf("expected %s [%s], got %v", label, want, got)
	}
}

func TestStageForDeletion_FromAggregateSelection(t *testing.T) {
	ctrl := newTestController(t, "gid1", "gid2", "gid3")

	manifest := stageForDeletion(t, ctrl, stageArgs{
		aggregates: stringSet("alice@example.com"),
	})

	if len(manifest.GmailIDs) != 3 {
		t.Errorf("expected 3 gmail IDs, got %d", len(manifest.GmailIDs))
	}
	assertSingleFilter(t, manifest.Filters.Senders, "alice@example.com", "senders")
	if manifest.CreatedBy != "tui" {
		t.Errorf("expected createdBy 'tui', got %q", manifest.CreatedBy)
	}
}

func TestStageForDeletion_FromMessageSelection(t *testing.T) {
	ctrl := newTestController(t)

	messages := msgSummaries("gid_a", "gid_b", "gid_c")

	manifest := stageForDeletion(t, ctrl, stageArgs{
		selection: idSet(1, 3),
		messages:  messages,
	})

	ids := make([]string, len(manifest.GmailIDs))
	copy(ids, manifest.GmailIDs)
	sort.Strings(ids)

	if len(ids) != 2 || ids[0] != "gid_a" || ids[1] != "gid_c" {
		t.Errorf("expected [gid_a gid_c], got %v", ids)
	}
}

func TestStageForDeletion_NoSelection(t *testing.T) {
	ctrl := newTestController(t)

	_, err := ctrl.StageForDeletion(
		nil, nil, query.ViewSenders, nil, nil,
		query.ViewSenders, "", query.TimeYear, nil,
	)
	if err == nil {
		t.Fatal("expected error for empty selection")
	}
}

func TestStageForDeletion_MultipleAggregates_DeterministicFilter(t *testing.T) {
	ctrl := newTestController(t, "gid1")

	agg := stringSet("charlie@example.com", "alice@example.com", "bob@example.com")

	for i := 0; i < 10; i++ {
		manifest := stageForDeletion(t, ctrl, stageArgs{aggregates: agg})
		if len(manifest.Filters.Senders) != 3 ||
			manifest.Filters.Senders[0] != "alice@example.com" ||
			manifest.Filters.Senders[1] != "bob@example.com" ||
			manifest.Filters.Senders[2] != "charlie@example.com" {
			t.Fatalf("iteration %d: expected sorted senders [alice bob charlie], got %v", i, manifest.Filters.Senders)
		}
	}
}

func TestStageForDeletion_ViewTypes(t *testing.T) {
	tests := []struct {
		name     string
		viewType query.ViewType
		key      string
		check    func(t *testing.T, f deletion.Filters)
	}{
		{"senders", query.ViewSenders, "a@b.com", func(t *testing.T, f deletion.Filters) {
			assertSingleFilter(t, f.Senders, "a@b.com", "senders")
		}},
		{"recipients", query.ViewRecipients, "c@d.com", func(t *testing.T, f deletion.Filters) {
			assertSingleFilter(t, f.Recipients, "c@d.com", "recipients")
		}},
		{"domains", query.ViewDomains, "example.com", func(t *testing.T, f deletion.Filters) {
			assertSingleFilter(t, f.SenderDomains, "example.com", "sender_domains")
		}},
		{"labels", query.ViewLabels, "INBOX", func(t *testing.T, f deletion.Filters) {
			assertSingleFilter(t, f.Labels, "INBOX", "labels")
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := newTestController(t, "gid1")

			manifest := stageForDeletion(t, ctrl, stageArgs{
				aggregates: stringSet(tt.key),
				view:       tt.viewType,
			})
			tt.check(t, manifest.Filters)
		})
	}
}

func TestStageForDeletion_AccountFilter(t *testing.T) {
	ctrl := newTestController(t, "gid1")

	accountID := int64(42)
	accounts := []query.AccountInfo{
		{ID: 42, Identifier: "test@gmail.com"},
	}

	manifest := stageForDeletion(t, ctrl, stageArgs{
		aggregates: stringSet("sender@x.com"),
		accountID:  &accountID,
		accounts:   accounts,
	})
	if manifest.Filters.Account != "test@gmail.com" {
		t.Errorf("expected account 'test@gmail.com', got %q", manifest.Filters.Account)
	}
}

func TestExportAttachments_NilDetail(t *testing.T) {
	ctrl := newTestController(t)
	cmd := ctrl.ExportAttachments(nil, nil)
	if cmd != nil {
		t.Error("expected nil cmd for nil detail")
	}
}

func TestExportAttachments_NoSelection(t *testing.T) {
	ctrl := newTestController(t)
	detail := &query.MessageDetail{
		Attachments: []query.AttachmentInfo{
			{ID: 1, Filename: "file.pdf", ContentHash: "abc123"},
		},
	}
	cmd := ctrl.ExportAttachments(detail, map[int]bool{})
	if cmd != nil {
		t.Error("expected nil cmd for empty selection")
	}
}
