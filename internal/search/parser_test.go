package search

import (
	"testing"
	"time"
)

func assertStrings(t *testing.T, field string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: len got %d, want %d (%v vs %v)", field, len(got), len(want), got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d]: got %q, want %q", field, i, got[i], want[i])
		}
	}
}

type parseTestCase struct {
	name        string
	query       string
	wantFrom    []string
	wantTo      []string
	wantText    []string
	wantSubject []string
	wantLabels  []string
	check       func(*testing.T, *Query)
}

func runParseTests(t *testing.T, tests []parseTestCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Parse(tt.query)
			assertStrings(t, "FromAddrs", q.FromAddrs, tt.wantFrom)
			assertStrings(t, "ToAddrs", q.ToAddrs, tt.wantTo)
			assertStrings(t, "TextTerms", q.TextTerms, tt.wantText)
			assertStrings(t, "SubjectTerms", q.SubjectTerms, tt.wantSubject)
			assertStrings(t, "Labels", q.Labels, tt.wantLabels)

			if tt.check != nil {
				tt.check(t, q)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []parseTestCase{
		// Basic Operators
		{
			name:     "from operator",
			query:    "from:alice@example.com",
			wantFrom: []string{"alice@example.com"},
		},
		{
			name:   "to operator",
			query:  "to:bob@example.com",
			wantTo: []string{"bob@example.com"},
		},
		{
			name:     "multiple from",
			query:    "from:alice@example.com from:bob@example.com",
			wantFrom: []string{"alice@example.com", "bob@example.com"},
		},
		{
			name:     "bare text",
			query:    "hello world",
			wantText: []string{"hello", "world"},
		},
		{
			name:     "quoted phrase",
			query:    `"hello world"`,
			wantText: []string{"hello world"},
		},
		{
			name:     "mixed operators and text",
			query:    "from:alice@example.com meeting notes",
			wantFrom: []string{"alice@example.com"},
			wantText: []string{"meeting", "notes"},
		},

		// Quoted Operator Values
		{
			name:        "subject with quoted phrase",
			query:       `subject:"meeting notes"`,
			wantSubject: []string{"meeting notes"},
		},
		{
			name:        "subject with quoted phrase and other terms",
			query:       `subject:"project update" from:alice@example.com`,
			wantSubject: []string{"project update"},
			wantFrom:    []string{"alice@example.com"},
		},
		{
			name:       "label with quoted value containing spaces",
			query:      `label:"My Important Label"`,
			wantLabels: []string{"My Important Label"},
		},
		{
			name:        "mixed quoted and unquoted",
			query:       `subject:urgent subject:"very important" search term`,
			wantSubject: []string{"urgent", "very important"},
			wantText:    []string{"search", "term"},
		},
		{
			name:     "from with quoted display name style (edge case)",
			query:    `from:"alice@example.com"`,
			wantFrom: []string{"alice@example.com"},
		},

		// Quoted Phrases With Colons
		{
			name:     "quoted phrase with colon",
			query:    `"foo:bar"`,
			wantText: []string{"foo:bar"},
		},
		{
			name:     "quoted phrase with time",
			query:    `"meeting at 10:30"`,
			wantText: []string{"meeting at 10:30"},
		},
		{
			name:     "quoted phrase with URL-like content",
			query:    `"check http://example.com"`,
			wantText: []string{"check http://example.com"},
		},
		{
			name:     "quoted phrase with multiple colons",
			query:    `"a:b:c:d"`,
			wantText: []string{"a:b:c:d"},
		},
		{
			name:     "quoted colon phrase mixed with real operator",
			query:    `from:alice@example.com "subject:not an operator"`,
			wantFrom: []string{"alice@example.com"},
			wantText: []string{"subject:not an operator"},
		},
		{
			name:     "operator followed by quoted colon phrase",
			query:    `"re: meeting notes" from:bob@example.com`,
			wantText: []string{"re: meeting notes"},
			wantFrom: []string{"bob@example.com"},
		},

		// Labels (Legacy dedicated test)
		{
			name:       "multiple labels",
			query:      "label:INBOX l:work",
			wantLabels: []string{"INBOX", "work"},
		},

		// Subject (Legacy dedicated test)
		{
			name:        "simple subject",
			query:       "subject:urgent",
			wantSubject: []string{"urgent"},
		},

		// Complex Query
		{
			name:        "complex query",
			query:       `from:alice@example.com to:bob@example.com subject:meeting has:attachment after:2024-01-01 "project report"`,
			wantFrom:    []string{"alice@example.com"},
			wantTo:      []string{"bob@example.com"},
			wantSubject: []string{"meeting"},
			wantText:    []string{"project report"},
			check: func(t *testing.T, q *Query) {
				if q.HasAttachment == nil || !*q.HasAttachment {
					t.Errorf("HasAttachment: expected true")
				}
				if q.AfterDate == nil {
					t.Errorf("AfterDate: expected not nil")
				}
			},
		},
	}

	runParseTests(t, tests)
}

func TestParse_HasAttachment(t *testing.T) {
	q := Parse("has:attachment")
	if q.HasAttachment == nil || !*q.HasAttachment {
		t.Errorf("HasAttachment: expected true, got %v", q.HasAttachment)
	}
}

func TestParse_Dates(t *testing.T) {
	q := Parse("after:2024-01-15 before:2024-06-30")

	if q.AfterDate == nil {
		t.Fatal("AfterDate is nil")
	}
	expectedAfter := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if !q.AfterDate.Equal(expectedAfter) {
		t.Errorf("AfterDate: got %v, want %v", q.AfterDate, expectedAfter)
	}

	if q.BeforeDate == nil {
		t.Fatal("BeforeDate is nil")
	}
	expectedBefore := time.Date(2024, 6, 30, 0, 0, 0, 0, time.UTC)
	if !q.BeforeDate.Equal(expectedBefore) {
		t.Errorf("BeforeDate: got %v, want %v", q.BeforeDate, expectedBefore)
	}
}

func TestParse_RelativeDates(t *testing.T) {
	q := Parse("newer_than:7d")
	if q.AfterDate == nil {
		t.Fatal("AfterDate is nil for newer_than")
	}

	// Should be approximately 7 days ago
	expected := time.Now().UTC().AddDate(0, 0, -7)
	diff := q.AfterDate.Sub(expected)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("AfterDate: got %v, expected around %v", q.AfterDate, expected)
	}
}

func TestParse_Sizes(t *testing.T) {
	tests := []struct {
		query string
		check func(q *Query) bool
	}{
		{
			query: "larger:5M",
			check: func(q *Query) bool {
				return q.LargerThan != nil && *q.LargerThan == 5*1024*1024
			},
		},
		{
			query: "smaller:100K",
			check: func(q *Query) bool {
				return q.SmallerThan != nil && *q.SmallerThan == 100*1024
			},
		},
		{
			query: "larger:1G",
			check: func(q *Query) bool {
				return q.LargerThan != nil && *q.LargerThan == 1024*1024*1024
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			q := Parse(tt.query)
			if !tt.check(q) {
				t.Errorf("Size filter not parsed correctly for %q", tt.query)
			}
		})
	}
}

func TestQuery_IsEmpty(t *testing.T) {
	tests := []struct {
		query   string
		isEmpty bool
	}{
		{"", true},
		{"from:alice@example.com", false},
		{"hello", false},
		{"has:attachment", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			q := Parse(tt.query)
			if q.IsEmpty() != tt.isEmpty {
				t.Errorf("IsEmpty(%q): got %v, want %v", tt.query, q.IsEmpty(), tt.isEmpty)
			}
		})
	}
}