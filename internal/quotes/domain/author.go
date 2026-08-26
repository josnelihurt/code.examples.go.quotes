package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Bounds for the quote author value object.
const (
	AuthorMinLength = 2
	AuthorMaxLength = 80
)

// Author is the quote's attribution: whitespace-normalized, 2–80 characters
// (counted in runes), only letters (any alphabet), spaces, hyphens,
// apostrophes (straight and curly), periods and combining marks. The zero
// value is not a valid Author.
type Author struct {
	Value string
}

// NewAuthor validates and normalizes raw input into an Author.
func NewAuthor(raw string) (Author, error) {
	normalized := normalizeWhitespace(raw)

	if utf8.RuneCountInString(normalized) < AuthorMinLength {
		return Author{}, AuthorTooShort()
	}

	if utf8.RuneCountInString(normalized) > AuthorMaxLength {
		return Author{}, AuthorTooLong()
	}

	if !isValidAuthor(normalized) {
		return Author{}, AuthorInvalidCharacters()
	}

	return Author{Value: normalized}, nil
}

// AuthorFromTrusted rebuilds an author already accepted by the catalog
// (seed/persistence). It skips create validation and only rejects blank input.
func AuthorFromTrusted(value string) (Author, error) {
	if strings.TrimSpace(value) == "" {
		return Author{}, errBlankTrustedValue("author")
	}
	return Author{Value: value}, nil
}

// isValidAuthor reports whether every rune is in the allowed set: letters,
// whitespace, hyphens, apostrophes (straight and curly), periods, and combining
// marks used in some Latin names (e.g. the U+0301 in a decomposed "José").
func isValidAuthor(author string) bool {
	for _, ch := range author {
		switch {
		case unicode.IsLetter(ch), unicode.IsSpace(ch):
		case ch == '-' || ch == '\'' || ch == '.' || ch == '’':
		case unicode.Is(unicode.Mn, ch):
		default:
			return false
		}
	}
	return true
}
