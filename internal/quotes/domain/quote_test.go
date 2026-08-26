package domain_test

import (
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewQuote(t *testing.T) {
	t.Run("accepts a well formed quote", func(t *testing.T) {
		quote, err := domain.NewQuote(
			"  Simplicity   is the ultimate sophistication.  ",
			"  Leonardo da Vinci  ")

		require.NoError(t, err)
		assert.Equal(t, "Simplicity is the ultimate sophistication.", quote.Text.Value)
		assert.Equal(t, "Leonardo da Vinci", quote.Author.Value)
		assert.Equal(t, "simplicity is the ultimate sophistication", quote.Fingerprint.Value)

		// The id mirrors the .NET Guid "N" format: 32 lowercase hex digits,
		// no dashes.
		assert.Regexp(t, `^[0-9a-f]{32}$`, quote.ID)
	})

	t.Run("mints distinct ids for identical input", func(t *testing.T) {
		first, err := domain.NewQuote("Talk is cheap. Show me the code.", "Linus Torvalds")
		require.NoError(t, err)
		second, err := domain.NewQuote("Talk is cheap. Show me the code.", "Linus Torvalds")
		require.NoError(t, err)

		assert.NotEqual(t, first.ID, second.ID)
	})

	t.Run("rejects author equal to text", func(t *testing.T) {
		const text = "Simple words make a point."

		_, err := domain.NewQuote(text, text)

		requireCode(t, err, "quote.author_equals_text")
	})

	t.Run("rejects author equal to text modulo case", func(t *testing.T) {
		_, err := domain.NewQuote("Talk is cheap. Show me the code.", "TALK IS CHEAP. SHOW ME THE CODE.")

		requireCode(t, err, "quote.author_equals_text")
	})

	t.Run("propagates text validation errors", func(t *testing.T) {
		_, err := domain.NewQuote("Too short.", "Ada Lovelace")

		requireCode(t, err, "quote.text_too_short")
	})

	t.Run("propagates author validation errors", func(t *testing.T) {
		_, err := domain.NewQuote("Talk is cheap. Show me the code.", "A")

		requireCode(t, err, "quote.author_too_short")
	})
}

func TestReconstituteQuote(t *testing.T) {
	t.Run("rebuilds a quote already accepted by the catalog", func(t *testing.T) {
		quote, err := domain.ReconstituteQuote(
			"7",
			"Programs must be written for people to read.",
			"Harold Abelson",
			"programs must be written for people to read")

		require.NoError(t, err)
		assert.Equal(t, "7", quote.ID)
		assert.Equal(t, "Programs must be written for people to read.", quote.Text.Value)
		assert.Equal(t, "Harold Abelson", quote.Author.Value)
		assert.Equal(t, "programs must be written for people to read", quote.Fingerprint.Value)
	})

	t.Run("skips create validation", func(t *testing.T) {
		quote, err := domain.ReconstituteQuote("7", "Short.", "A", "x")

		require.NoError(t, err)
		assert.Equal(t, "Short.", quote.Text.Value)
		assert.Equal(t, "A", quote.Author.Value)
	})

	t.Run("rejects a blank id", func(t *testing.T) {
		_, err := domain.ReconstituteQuote(
			"   ",
			"Programs must be written for people to read.",
			"Harold Abelson",
			"programs must be written for people to read")

		require.Error(t, err)
	})

	t.Run("rejects an empty id", func(t *testing.T) {
		_, err := domain.ReconstituteQuote(
			"",
			"Programs must be written for people to read.",
			"Harold Abelson",
			"programs must be written for people to read")

		require.Error(t, err)
	})

	t.Run("rejects a blank text", func(t *testing.T) {
		_, err := domain.ReconstituteQuote("7", "  ", "Harold Abelson", "fingerprint")

		require.Error(t, err)
	})

	t.Run("rejects a blank author", func(t *testing.T) {
		_, err := domain.ReconstituteQuote("7", "Programs must be written for people to read.", "", "fingerprint")

		require.Error(t, err)
	})

	t.Run("rejects a blank fingerprint", func(t *testing.T) {
		_, err := domain.ReconstituteQuote(
			"7",
			"Programs must be written for people to read.",
			"Harold Abelson",
			"  ")

		require.Error(t, err)
	})
}
