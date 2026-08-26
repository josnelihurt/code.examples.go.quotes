package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/require"
)

// The canonical constructors return a fresh pointer per call, so identity has
// to come from the Code rather than the pointer — otherwise errors.Is against
// a constructor could never match and the port's not-found contract would be
// unusable.
func TestErrorIsComparesByCode(t *testing.T) {
	// testifylint reads the two NotFound() calls as the same expression and
	// flags a useless assert; they are two independently constructed pointers,
	// and that they compare equal is precisely what Is exists to make true.
	//nolint:testifylint // distinct values from one constructor, not one value
	require.ErrorIs(t, domain.NotFound(), domain.NotFound(),
		"two separately constructed not-found errors must compare equal")

	require.NotErrorIs(t, domain.NotFound(), domain.DuplicateFingerprint(),
		"different codes must not compare equal")

	require.ErrorIs(t, fmt.Errorf("reading the catalog: %w", domain.NotFound()), domain.NotFound(),
		"a wrapped domain error must still match its code")

	require.NotErrorIs(t, errors.New("quote.not_found"), domain.NotFound(),
		"a plain error that merely spells the code is not the domain error")
}

// The Description is deliberately outside the identity: rewording a message
// must never change which errors compare equal.
func TestErrorIsIgnoresDescription(t *testing.T) {
	reworded := &domain.Error{Code: domain.NotFound().Code, Description: "Something else entirely."}

	require.ErrorIs(t, reworded, domain.NotFound())
}

// errors.As must keep working for the transport, which reads Code off the
// concrete type to pick a status.
func TestErrorAsStillExposesTheCode(t *testing.T) {
	var domainErr *domain.Error

	require.ErrorAs(t, fmt.Errorf("wrapped: %w", domain.DuplicateFingerprint()), &domainErr)
	require.Equal(t, "quote.duplicate_fingerprint", domainErr.Code)
}
