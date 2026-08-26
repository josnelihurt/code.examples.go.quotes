package domain_test

import (
	"strings"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuthor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantCode string
	}{
		{
			name:  "normalizes whitespace",
			input: "  Leonardo da Vinci  ",
			want:  "Leonardo da Vinci",
		},
		{
			name:     "rejects an author that is too short",
			input:    "A",
			wantCode: "quote.author_too_short",
		},
		{
			name:  "accepts a two character author",
			input: "Al",
			want:  "Al",
		},
		{
			name:     "rejects an author that is too long",
			input:    strings.Repeat("a", domain.MaxAuthorLength+1),
			wantCode: "quote.author_too_long",
		},
		{
			name:  "accepts an eighty character author",
			input: strings.Repeat("a", domain.MaxAuthorLength),
			want:  strings.Repeat("a", domain.MaxAuthorLength),
		},
		{
			name:     "rejects author with digits",
			input:    "Author 42",
			wantCode: "quote.author_invalid_characters",
		},
		{
			name:     "rejects author with punctuation outside the allowed set",
			input:    "Author, Jr.",
			wantCode: "quote.author_invalid_characters",
		},
		{
			name:     "rejects author with an at sign",
			input:    "user@example",
			wantCode: "quote.author_invalid_characters",
		},
		{
			name:  "accepts unicode letters",
			input: "José Ortega y Gasset",
			want:  "José Ortega y Gasset",
		},
		{
			name:  "accepts hyphens",
			input: "Mary-Jane Doe",
			want:  "Mary-Jane Doe",
		},
		{
			name:  "accepts apostrophes",
			input: "Anthony O'Brien",
			want:  "Anthony O'Brien",
		},
		{
			name:  "accepts curly apostrophes",
			input: "Pete’s Craft",
			want:  "Pete’s Craft",
		},
		{
			name:  "accepts periods",
			input: "Martin Luther King Jr.",
			want:  "Martin Luther King Jr.",
		},
		{
			// "Jose" + U+0301 combining acute and "Marti" + U+0301: the
			// NonSpacingMark category is allowed for decomposed Latin names.
			name:  "accepts combining marks",
			input: "Jose\u0301 Marti\u0301",
			want:  "Jose\u0301 Marti\u0301",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			author, err := domain.NewAuthor(test.input)

			if test.wantCode != "" {
				require.Error(t, err)
				requireCode(t, err, test.wantCode)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.want, author.Value)
		})
	}
}

func TestAuthorFromTrusted(t *testing.T) {
	t.Run("rebuilds a catalog author without create validation", func(t *testing.T) {
		author, err := domain.AuthorFromTrusted("A")

		require.NoError(t, err)
		assert.Equal(t, "A", author.Value)
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
			_, err := domain.AuthorFromTrusted(test.value)

			require.Error(t, err)
		})
	}
}
