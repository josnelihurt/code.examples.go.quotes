package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/assert"
)

// The canonical constructors return a fresh pointer per call, so identity has
// to come from the Code rather than the pointer — otherwise errors.Is against
// a constructor could never match and the port's not-found contract would be
// unusable.
func TestErrorIsComparesByCode(t *testing.T) {
	assert.True(t, errors.Is(domain.NotFound(), domain.NotFound()),
		"two separately constructed not-found errors must compare equal")

	assert.False(t, errors.Is(domain.NotFound(), domain.DuplicateFingerprint()),
		"different codes must not compare equal")

	assert.True(t, errors.Is(fmt.Errorf("reading the catalog: %w", domain.NotFound()), domain.NotFound()),
		"a wrapped domain error must still match its code")

	assert.False(t, errors.Is(errors.New("quote.not_found"), domain.NotFound()),
		"a plain error that merely spells the code is not the domain error")
}

// The Description is deliberately outside the identity: rewording a message
// must never change which errors compare equal.
func TestErrorIsIgnoresDescription(t *testing.T) {
	reworded := &domain.Error{Code: domain.NotFound().Code, Description: "Something else entirely."}

	assert.True(t, errors.Is(reworded, domain.NotFound()))
}

// errors.As must keep working for the transport, which reads Code off the
// concrete type to pick a status.
func TestErrorAsStillExposesTheCode(t *testing.T) {
	var domainErr *domain.Error

	assert.True(t, errors.As(fmt.Errorf("wrapped: %w", domain.DuplicateFingerprint()), &domainErr))
	assert.Equal(t, "quote.duplicate_fingerprint", domainErr.Code)
}
