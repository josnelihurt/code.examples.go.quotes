package domain

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// Quote is the catalog's aggregate: a validated text, its author, the
// fingerprint derived from the text, and the id assigned at creation.
type Quote struct {
	ID          string
	Text        Text
	Author      Author
	Fingerprint Fingerprint
}

// NewQuote creates a quote from raw input, validating the text and the author
// and refusing an author that repeats the text (case-insensitively). The id is
// a fresh GUID in "N" format — 32 lowercase hex digits, no dashes — mirroring
// the .NET catalog's Guid.NewGuid().ToString("N") shape.
func NewQuote(text, author string) (*Quote, error) {
	textValue, err := NewText(text)
	if err != nil {
		return nil, err
	}

	authorValue, err := NewAuthor(author)
	if err != nil {
		return nil, err
	}

	if strings.EqualFold(textValue.Value, authorValue.Value) {
		return nil, AuthorEqualsText()
	}

	return &Quote{
		ID:          newQuoteID(),
		Text:        textValue,
		Author:      authorValue,
		Fingerprint: FingerprintFromText(textValue),
	}, nil
}

// ReconstituteQuote rebuilds a quote already accepted by the catalog
// (seed/persistence): no create validation, but every component must be
// non-blank so a mis-wired reconstitution fails loudly instead of seeding the
// catalog with empty rows.
func ReconstituteQuote(id, text, author, fingerprint string) (*Quote, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errBlankTrustedValue("id")
	}

	textValue, err := TextFromTrusted(text)
	if err != nil {
		return nil, err
	}

	authorValue, err := AuthorFromTrusted(author)
	if err != nil {
		return nil, err
	}

	fingerprintValue, err := FingerprintFromTrusted(fingerprint)
	if err != nil {
		return nil, err
	}

	return &Quote{
		ID:          id,
		Text:        textValue,
		Author:      authorValue,
		Fingerprint: fingerprintValue,
	}, nil
}

// newQuoteID mints the 32-hex-digit, lowercase, dash-free GUID "N" form.
func newQuoteID() string {
	var bytes [16]byte
	// crypto/rand.Read is guaranteed to fill the buffer or panic (Go 1.24+);
	// an unrecoverable entropy failure is not an error a caller can handle.
	_, _ = rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:])
}
