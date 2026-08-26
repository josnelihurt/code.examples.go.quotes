package domain_test

import (
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeFingerprint(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "lowercases and drops terminal punctuation",
			text: "Code is like humor!",
			want: "code is like humor",
		},
		{
			name: "ignores punctuation differences",
			text: "code is like humor.",
			want: "code is like humor",
		},
		{
			// Punctuation drops out but keeps a word break, so "First,solve"
			// fingerprints as "first solve".
			name: "turns inner punctuation into word breaks",
			text: "First,solve. Then,write.",
			want: "first solve then write",
		},
		{
			name: "keeps digits",
			text: "There are 10 types of people!",
			want: "there are 10 types of people",
		},
		{
			name: "collapses whitespace runs",
			text: "  Talk   is\tcheap.  ",
			want: "talk is cheap",
		},
		{
			name: "fingerprint of empty input is empty",
			text: "",
			want: "",
		},
		{
			name: "fingerprint of punctuation only is empty",
			text: "!!! ???",
			want: "",
		},
		{
			name: "lowercases unicode letters",
			text: "Ünïcödé wörks!",
			want: "ünïcödé wörks",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, domain.ComputeFingerprint(test.text))
		})
	}
}

func TestFingerprintFromText(t *testing.T) {
	t.Run("matches the compute fingerprint on the text value", func(t *testing.T) {
		text, err := domain.NewText("Talk is cheap. Show me the code.")
		require.NoError(t, err)

		fingerprint := domain.FingerprintFromText(text)

		assert.Equal(t, text.Fingerprint(), fingerprint.Value)
		assert.Equal(t, "talk is cheap show me the code", fingerprint.Value)
	})
}

func TestFingerprintFromTrusted(t *testing.T) {
	t.Run("rebuilds a stored fingerprint", func(t *testing.T) {
		fingerprint, err := domain.FingerprintFromTrusted("first solve then write")

		require.NoError(t, err)
		assert.Equal(t, "first solve then write", fingerprint.Value)
	})

	tests := []struct {
		name  string
		value string
	}{
		{name: "rejects an empty value", value: ""},
		{name: "rejects a blank value", value: "  "},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := domain.FingerprintFromTrusted(test.value)

			require.Error(t, err)
		})
	}
}
