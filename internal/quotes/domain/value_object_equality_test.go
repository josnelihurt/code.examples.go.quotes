package domain_test

import (
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The glossary promises value objects compare by value; these tests keep the
// comparability honest for all three.
func TestValueObjectsWithTheSameValueAreEqual(t *testing.T) {
	tests := []struct {
		name  string
		left  any
		right any
	}{
		{
			name:  "text",
			left:  mustText(t, "Programs must be written for people to read."),
			right: mustText(t, "Programs must be written for people to read."),
		},
		{
			name:  "author",
			left:  mustAuthor(t, "Harold Abelson"),
			right: mustAuthor(t, "Harold Abelson"),
		},
		{
			name:  "fingerprint",
			left:  mustFingerprint(t, "first solve then write"),
			right: mustFingerprint(t, "first solve then write"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.left, test.right)
		})
	}
}

func TestValueObjectsWithDifferentValuesAreNotEqual(t *testing.T) {
	tests := []struct {
		name  string
		left  any
		right any
	}{
		{
			name:  "text",
			left:  mustText(t, "Programs must be written for people to read."),
			right: mustText(t, "Everything should be made as simple as possible."),
		},
		{
			name:  "author",
			left:  mustAuthor(t, "Harold Abelson"),
			right: mustAuthor(t, "Albert Einstein"),
		},
		{
			name:  "fingerprint",
			left:  mustFingerprint(t, "first solve"),
			right: mustFingerprint(t, "then write"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.NotEqual(t, test.left, test.right)
		})
	}
}

func TestValueObjectsHashEquallyAndWorkAsMapKeys(t *testing.T) {
	texts := map[domain.Text]int{
		mustText(t, "Programs must be written for people to read."): 1,
	}
	texts[mustText(t, "Programs must be written for people to read.")]++

	assert.Equal(t, 1, len(texts))
	assert.Equal(t, 2, texts[mustText(t, "Programs must be written for people to read.")])
}

func mustText(t *testing.T, value string) domain.Text {
	t.Helper()
	text, err := domain.TextFromTrusted(value)
	require.NoError(t, err)
	return text
}

func mustAuthor(t *testing.T, value string) domain.Author {
	t.Helper()
	author, err := domain.AuthorFromTrusted(value)
	require.NoError(t, err)
	return author
}

func mustFingerprint(t *testing.T, value string) domain.Fingerprint {
	t.Helper()
	fingerprint, err := domain.FingerprintFromTrusted(value)
	require.NoError(t, err)
	return fingerprint
}
