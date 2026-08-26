package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Bounds for the quote text value object.
const (
	TextMinLength    = 12
	TextMaxLength    = 280
	TextMinWordCount = 3
)

// Text is the quote's body: whitespace-normalized, 12–280 characters (counted
// in runes — .NET counts UTF-16 code units, which diverges only for astral
// characters), at least three words, and ending with sentence punctuation.
// The zero value is not a valid Text.
type Text struct {
	Value string
}

// NewText validates and normalizes raw input into a Text.
func NewText(raw string) (Text, error) {
	normalized := normalizeWhitespace(raw)

	if utf8.RuneCountInString(normalized) < TextMinLength {
		return Text{}, TextTooShort()
	}

	if utf8.RuneCountInString(normalized) > TextMaxLength {
		return Text{}, TextTooLong()
	}

	if len(splitWords(normalized)) < TextMinWordCount {
		return Text{}, TextNeedsMoreWords()
	}

	last, _ := utf8.DecodeLastRuneInString(normalized)
	if last != '.' && last != '!' && last != '?' {
		return Text{}, TextMustEndWithPunctuation()
	}

	return Text{Value: normalized}, nil
}

// TextFromTrusted rebuilds text already accepted by the catalog
// (seed/persistence). It skips create validation and only rejects blank input.
func TextFromTrusted(value string) (Text, error) {
	if strings.TrimSpace(value) == "" {
		return Text{}, errBlankTrustedValue("text")
	}
	return Text{Value: value}, nil
}

// Fingerprint returns the normalized fingerprint of the validated text.
func (t Text) Fingerprint() string {
	return ComputeFingerprint(t.Value)
}

// ComputeFingerprint fingerprints raw input (e.g. seed helpers) without going
// through full create validation: lowercase, letters and digits kept, every
// other character treated as a word break, single spaces between words.
func ComputeFingerprint(text string) string {
	normalized := strings.ToLower(normalizeWhitespace(text))
	var builder strings.Builder
	pendingSpace := false

	for _, ch := range normalized {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) {
			if pendingSpace && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(ch)
			pendingSpace = false
			continue
		}
		// Whitespace and punctuation both drop out but keep a word break, so
		// "First,solve" fingerprints as "first solve".
		pendingSpace = true
	}

	return builder.String()
}

// errBlankTrustedValue guards the FromTrusted/Reconstitute constructors: a
// value the catalog already accepted can still be a blank string when callers
// wire the reconstitution wrong, and that must fail loudly instead of seeding
// the catalog with empty rows.
func errBlankTrustedValue(what string) error {
	return fmt.Errorf("trusted %s must not be blank", what)
}

// normalizeWhitespace collapses all whitespace runs into single spaces and
// trims the ends; blank input normalizes to the empty string.
func normalizeWhitespace(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return strings.Join(strings.Fields(value), " ")
}

// splitWords splits whitespace-normalized text into words.
func splitWords(text string) []string {
	return strings.Fields(text)
}
