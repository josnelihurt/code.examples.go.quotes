// Package domain holds the quotes bounded context's core model: the Quote
// aggregate, its value objects, the canonical error codes, the paging rules and
// the repository port that adapters implement. It imports nothing from outside
// the standard library.
//
// Failures are *domain.Error values carrying a stable, wire-visible Code — the
// codes are part of the public API contract (they surface as the errorCode
// field of error envelopes), so renaming one is a breaking change. The
// transport layer maps Code to an HTTP status (quote.not_found → 404,
// quote.duplicate_fingerprint → 409, quote.invalid_page_request → 400, ...);
// this package knows nothing about transport.
package domain

import "fmt"

// Error is a canonical domain failure: a stable code the transport maps to an
// HTTP status, plus a human-readable description.
type Error struct {
	Code        string
	Description string
}

// Error implements the error interface; the message carries both the code and
// the description so logs can grep either.
func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Description)
}

// TextTooShort returns the canonical error for text below the minimum length.
func TextTooShort() *Error {
	return &Error{
		Code:        "quote.text_too_short",
		Description: fmt.Sprintf("Quote text must be at least %d characters.", TextMinLength),
	}
}

// TextTooLong returns the canonical error for text above the maximum length.
func TextTooLong() *Error {
	return &Error{
		Code:        "quote.text_too_long",
		Description: fmt.Sprintf("Quote text must be at most %d characters.", TextMaxLength),
	}
}

// TextNeedsMoreWords returns the canonical error for text with too few words.
func TextNeedsMoreWords() *Error {
	return &Error{
		Code:        "quote.text_needs_more_words",
		Description: fmt.Sprintf("Quote text must contain at least %d words.", TextMinWordCount),
	}
}

// TextMustEndWithPunctuation returns the canonical error for text without a
// sentence terminator.
func TextMustEndWithPunctuation() *Error {
	return &Error{
		Code:        "quote.text_must_end_with_punctuation",
		Description: "Quote text must end with '.', '!', or '?'.",
	}
}

// AuthorTooShort returns the canonical error for an author below the minimum
// length.
func AuthorTooShort() *Error {
	return &Error{
		Code:        "quote.author_too_short",
		Description: fmt.Sprintf("Author must be at least %d characters.", AuthorMinLength),
	}
}

// AuthorTooLong returns the canonical error for an author above the maximum
// length.
func AuthorTooLong() *Error {
	return &Error{
		Code:        "quote.author_too_long",
		Description: fmt.Sprintf("Author must be at most %d characters.", AuthorMaxLength),
	}
}

// AuthorInvalidCharacters returns the canonical error for an author with
// characters outside the allowed set.
func AuthorInvalidCharacters() *Error {
	return &Error{
		Code:        "quote.author_invalid_characters",
		Description: "Author may only contain letters (any alphabet), spaces, hyphens, apostrophes, and periods.",
	}
}

// AuthorEqualsText returns the canonical error for an author that repeats the
// quote text.
func AuthorEqualsText() *Error {
	return &Error{
		Code:        "quote.author_equals_text",
		Description: "Author must not be the same as the quote text.",
	}
}

// NotFound returns the canonical error for a missing quote.
func NotFound() *Error {
	return &Error{
		Code:        "quote.not_found",
		Description: "Quote not found.",
	}
}

// InvalidPageRequest returns the canonical error for paging parameters outside
// the allowed range.
func InvalidPageRequest() *Error {
	return &Error{
		Code:        "quote.invalid_page_request",
		Description: "The requested page or page size is outside the allowed range.",
	}
}

// DuplicateFingerprint returns the canonical error for a quote whose
// fingerprint already exists in the catalog.
func DuplicateFingerprint() *Error {
	return &Error{
		Code:        "quote.duplicate_fingerprint",
		Description: "A quote with the same meaning already exists.",
	}
}
