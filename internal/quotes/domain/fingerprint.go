package domain

import "strings"

// Fingerprint is the quote's deduplication key: the quote text lowercased with
// everything but letters and digits turned into single-space word breaks. Two
// quotes with the same fingerprint are "the same meaning" to the catalog. The
// zero value is not a valid Fingerprint.
type Fingerprint struct {
	Value string
}

// FingerprintFromText derives the fingerprint of a validated text.
func FingerprintFromText(text Text) Fingerprint {
	return Fingerprint{Value: text.Fingerprint()}
}

// FingerprintFromTrusted rebuilds a fingerprint already stored by the catalog
// (seed/persistence). It only rejects blank input.
func FingerprintFromTrusted(value string) (Fingerprint, error) {
	if strings.TrimSpace(value) == "" {
		return Fingerprint{}, errBlankTrustedValue("fingerprint")
	}
	return Fingerprint{Value: value}, nil
}
