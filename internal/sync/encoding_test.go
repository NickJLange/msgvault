package sync

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEnsureUTF8_AlreadyValid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ASCII", "Hello, World!"},
		{"UTF-8 Chinese", "你好世界"},
		{"UTF-8 Japanese", "こんにちは"},
		{"UTF-8 Korean", "안녕하세요"},
		{"UTF-8 Cyrillic", "Привет мир"},
		{"UTF-8 mixed", "Hello 世界! Привет!"},
		{"UTF-8 emoji", "Hello 👋 World 🌍"},
		{"empty string", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureUTF8(tt.input)
			if result != tt.input {
				t.Errorf("ensureUTF8(%q) = %q, want unchanged", tt.input, result)
			}
		})
	}
}

func TestEnsureUTF8_Windows1252(t *testing.T) {
	// Windows-1252 specific characters that differ from Latin-1
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "smart single quote (right)",
			input:    []byte("Rand\x92s Opponent"), // 0x92 = ' (U+2019)
			expected: "Rand\u2019s Opponent",
		},
		{
			name:     "en dash",
			input:    []byte("2020 \x96 2024"), // 0x96 = – (U+2013)
			expected: "2020 \u2013 2024",
		},
		{
			name:     "em dash",
			input:    []byte("Hello\x97World"), // 0x97 = — (U+2014)
			expected: "Hello\u2014World",
		},
		{
			name:     "left double quote",
			input:    []byte("\x93Hello\x94"), // 0x93/" 0x94/"
			expected: "\u201cHello\u201d",
		},
		{
			name:     "trademark",
			input:    []byte("Brand\x99"), // 0x99 = ™
			expected: "Brand\u2122",
		},
		{
			name:     "bullet",
			input:    []byte("\x95 Item"), // 0x95 = •
			expected: "\u2022 Item",
		},
		{
			name:     "euro sign",
			input:    []byte("Price: \x80100"), // 0x80 = €
			expected: "Price: \u20ac100",
		},
	}

	runEncodingTests(t, tests)
}

func TestEnsureUTF8_Latin1(t *testing.T) {
	// ISO-8859-1 (Latin-1) characters
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "o with acute",
			input:    []byte("Mir\xf3 - Picasso"), // ó
			expected: "Miró - Picasso",
		},
		{
			name:     "c with cedilla",
			input:    []byte("Gar\xe7on"), // ç
			expected: "Garçon",
		},
		{
			name:     "u with umlaut",
			input:    []byte("M\xfcnchen"), // ü
			expected: "München",
		},
		{
			name:     "n with tilde",
			input:    []byte("Espa\xf1a"), // ñ
			expected: "España",
		},
		{
			name:     "registered trademark",
			input:    []byte("Laguiole.com \xae"), // ®
			expected: "Laguiole.com ®",
		},
		{
			name:     "degree symbol",
			input:    []byte("25\xb0C"), // °
			expected: "25°C",
		},
	}

	runEncodingTests(t, tests)
}

func TestEnsureUTF8_AsianEncodings(t *testing.T) {
	// For short Asian text samples, exact charset detection is unreliable
	// The key requirement is that output is valid UTF-8 and non-empty
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "Shift-JIS Japanese",
			input: []byte{0x82, 0xb1, 0x82, 0xf1, 0x82, 0xc9, 0x82, 0xbf, 0x82, 0xcd}, // こんにちは
		},
		{
			name:  "GBK Simplified Chinese",
			input: []byte{0xc4, 0xe3, 0xba, 0xc3}, // 你好
		},
		{
			name:  "Big5 Traditional Chinese",
			input: []byte{0xa9, 0x6f, 0xa6, 0x6e}, // 你好
		},
		{
			name:  "EUC-KR Korean",
			input: []byte{0xbe, 0xc8, 0xb3, 0xe7}, // 안녕
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureUTF8(string(tt.input))
			if !utf8.ValidString(result) {
				t.Errorf("result is not valid UTF-8: %q", result)
			}
			if len(result) == 0 {
				t.Errorf("result is empty")
			}
		})
	}
}

func TestEnsureUTF8_MixedContent(t *testing.T) {
	// Real-world scenario: ASCII mixed with encoded characters
	tests := []struct {
		name     string
		input    []byte
		contains []string // Substrings that should be present
	}{
		{
			name:     "email subject with smart quotes",
			input:    []byte("Re: Can\x92t access the \x93dashboard\x94"),
			contains: []string{"Re:", "Can", "access the", "dashboard"},
		},
		{
			name:     "price with currency",
			input:    []byte("Only \x80199.99 \x96 Limited Time"),
			contains: []string{"Only", "199.99", "Limited Time"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureUTF8(string(tt.input))
			if !utf8.ValidString(result) {
				t.Errorf("result is not valid UTF-8: %q", result)
			}
			for _, substr := range tt.contains {
				if !strings.Contains(result, substr) {
					t.Errorf("result %q should contain %q", result, substr)
				}
			}
		})
	}
}

func TestSanitizeUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid UTF-8 unchanged",
			input:    "Hello, 世界!",
			expected: "Hello, 世界!",
		},
		{
			name:     "single invalid byte",
			input:    "Hello\x80World",
			expected: "Hello\ufffdWorld",
		},
		{
			name:     "multiple invalid bytes",
			input:    "Test\x80\x81\x82String",
			expected: "Test\ufffd\ufffd\ufffdString",
		},
		{
			name:     "truncated UTF-8 sequence",
			input:    "Hello\xc3", // Incomplete 2-byte sequence
			expected: "Hello\ufffd",
		},
		{
			name:     "invalid continuation byte",
			input:    "Test\xc3\x00End", // Invalid continuation
			expected: "Test\ufffd\x00End",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeUTF8(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeUTF8(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			if !utf8.ValidString(result) {
				t.Errorf("result is not valid UTF-8")
			}
		})
	}
}

func TestGetEncodingByName(t *testing.T) {
	tests := []struct {
		charset string
		wantNil bool
	}{
		{"windows-1252", false},
		{"CP1252", false},
		{"ISO-8859-1", false},
		{"iso-8859-1", false},
		{"latin1", false},
		{"Shift_JIS", false},
		{"shift_jis", false},
		{"EUC-JP", false},
		{"EUC-KR", false},
		{"GBK", false},
		{"GB2312", false},
		{"Big5", false},
		{"KOI8-R", false},
		{"unknown-charset", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.charset, func(t *testing.T) {
			result := getEncodingByName(tt.charset)
			if tt.wantNil && result != nil {
				t.Errorf("getEncodingByName(%q) = %v, want nil", tt.charset, result)
			}
			if !tt.wantNil && result == nil {
				t.Errorf("getEncodingByName(%q) = nil, want encoding", tt.charset)
			}
		})
	}
}

// Helper to run table-driven encoding tests
func runEncodingTests(t *testing.T, tests []struct {
	name     string
	input    []byte
	expected string
}) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ensureUTF8(string(tt.input))
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
			if !utf8.ValidString(result) {
				t.Errorf("result is not valid UTF-8")
			}
		})
	}
}
