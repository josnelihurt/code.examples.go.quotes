package domain_test

import (
	"strings"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireCode asserts the error is a canonical domain error with the exact code.
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainErr *domain.Error
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, code, domainErr.Code)
}

func TestNewText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantCode string
	}{
		{
			name:  "normalizes whitespace",
			input: "  Simplicity   is the ultimate sophistication.  ",
			want:  "Simplicity is the ultimate sophistication.",
		},
		{
			name:     "rejects empty input",
			input:    "",
			wantCode: "quote.text_too_short",
		},
		{
			name:     "rejects blank input",
			input:    "   ",
			wantCode: "quote.text_too_short",
		},
		{
			name:     "rejects text below the minimum length",
			input:    "Too short.",
			wantCode: "quote.text_too_short",
		},
		{
			name:     "rejects eleven characters",
			input:    "a b c d e f",
			wantCode: "quote.text_too_short",
		},
		{
			name:  "accepts twelve characters",
			input: "a b c d e f!",
			want:  "a b c d e f!",
		},
		{
			name:     "rejects text above the maximum length",
			input:    strings.Repeat("a", domain.MaxTextLength+1) + ".",
			wantCode: "quote.text_too_long",
		},
		{
			name:     "rejects two hundred eighty one characters",
			input:    strings.Repeat("word ", 55) + "endss.",
			wantCode: "quote.text_too_long",
		},
		{
			name:  "accepts two hundred eighty characters",
			input: strings.Repeat("word ", 55) + "ends.",
			want:  strings.Repeat("word ", 55) + "ends.",
		},
		{
			// 11 runes but 22 bytes: length is counted in runes (the Go-side
			// rule; .NET's UTF-16 count agrees for BMP characters).
			name:     "counts runes not bytes below the minimum",
			input:    strings.Repeat("é", 11),
			wantCode: "quote.text_too_short",
		},
		{
			// 280 runes but 490 bytes: rune counting keeps this within the cap.
			name:  "counts runes not bytes at the maximum",
			input: strings.Repeat("ééé ", 69) + "ééé.",
			want:  strings.Repeat("ééé ", 69) + "ééé.",
		},
		{
			name:     "rejects text with fewer than three words",
			input:    "Hello world!",
			wantCode: "quote.text_needs_more_words",
		},
		{
			name:  "accepts exactly three words",
			input: "Hello big world.",
			want:  "Hello big world.",
		},
		{
			name:     "rejects text without terminal punctuation",
			input:    "Programs must be written for people to read",
			wantCode: "quote.text_must_end_with_punctuation",
		},
		{
			name:  "accepts a period terminator",
			input: "Talk is cheap. Show me the code.",
			want:  "Talk is cheap. Show me the code.",
		},
		{
			name:  "accepts an exclamation terminator",
			input: "Code is like humor!",
			want:  "Code is like humor!",
		},
		{
			name:  "accepts a question terminator",
			input: "Is it readable yet?",
			want:  "Is it readable yet?",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text, err := domain.NewText(test.input)

			if test.wantCode != "" {
				require.Error(t, err)
				requireCode(t, err, test.wantCode)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, text.Value)
		})
	}
}

func TestTextFromTrusted(t *testing.T) {
	t.Run("rebuilds catalog text without create validation", func(t *testing.T) {
		text, err := domain.TextFromTrusted("Trusted.")

		require.NoError(t, err)
		assert.Equal(t, "Trusted.", text.Value)
	})

	tests := []struct {
		name  string
		value string
	}{
		{name: "rejects an empty value", value: ""},
		{name: "rejects a blank value", value: "   "},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.TextFromTrusted(test.value)

			require.Error(t, err)
		})
	}
}
