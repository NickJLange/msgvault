package search

import (
	"testing"
	"time"
)

func utcDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func assertHasAttachment(t *testing.T, q *Query, want bool) {
	t.Helper()
	if want {
		if q.HasAttachment == nil || !*q.HasAttachment {
			t.Errorf("HasAttachment: expected true, got %v", q.HasAttachment)
		}
	} else {
		if q.HasAttachment != nil && *q.HasAttachment {
			t.Errorf("HasAttachment: expected false/nil, got %v", *q.HasAttachment)
		}
	}
}

func assertTimeEqual(t *testing.T, field string, got *time.Time, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: expected %v, got nil", field, want)
	}
	if !got.Equal(want) {
		t.Errorf("%s: got %v, want %v", field, *got, want)
	}
}

func assertTimeWithin(t *testing.T, field string, got *time.Time, want time.Time, tol time.Duration) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: expected around %v, got nil", field, want)
	}
	diff := got.Sub(want)
	if diff < -tol || diff > tol {
		t.Errorf("%s: got %v, expected within %v of %v", field, *got, tol, want)
	}
}

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
				assertHasAttachment(t, q, true)
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
	assertHasAttachment(t, q, true)
}

func TestParse_Dates(t *testing.T) {
	q := Parse("after:2024-01-15 before:2024-06-30")
	assertTimeEqual(t, "AfterDate", q.AfterDate, utcDate(2024, 1, 15))
	assertTimeEqual(t, "BeforeDate", q.BeforeDate, utcDate(2024, 6, 30))
}

func TestParse_RelativeDates(t *testing.T) {
	q := Parse("newer_than:7d")
	expected := time.Now().UTC().AddDate(0, 0, -7)
	assertTimeWithin(t, "AfterDate", q.AfterDate, expected, time.Second)
}

func TestParse_Sizes(t *testing.T) {
	type sizeCase struct {
		query   string
		larger  *int64
		smaller *int64
	}
	i64 := func(v int64) *int64 { return &v }

	tests := []sizeCase{
		{query: "larger:5M", larger: i64(5 * 1024 * 1024)},
		{query: "smaller:100K", smaller: i64(100 * 1024)},
		{query: "larger:1G", larger: i64(1024 * 1024 * 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			q := Parse(tt.query)
			if tt.larger != nil {
				if q.LargerThan == nil || *q.LargerThan != *tt.larger {
					t.Errorf("LargerThan: got %v, want %d", q.LargerThan, *tt.larger)
				}
			}
			if tt.smaller != nil {
				if q.SmallerThan == nil || *q.SmallerThan != *tt.smaller {
					t.Errorf("SmallerThan: got %v, want %d", q.SmallerThan, *tt.smaller)
				}
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