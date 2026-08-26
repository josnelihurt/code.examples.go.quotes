package application_test

import (
	"context"
	"testing"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/stretchr/testify/require"
)

// stubListCall records one List invocation.
type stubListCall struct {
	skip int
	take int
}

// stubQuoteRepository is the hand-written double for the repository port: the
// interface is four methods, so a fake with recorded calls beats generated
// mocks (ADR 0008).
type stubQuoteRepository struct {
	random    *domain.Quote
	randomErr error

	byID    map[string]*domain.Quote
	byIDErr error

	page    domain.QuotePage
	listErr error

	outcome domain.QuoteAddOutcome
	addErr  error

	getRandomCalls int
	getByIDCalls   int
	listCalls      []stubListCall
	addCalls       []*domain.Quote
}

func (s *stubQuoteRepository) GetRandom(context.Context) (*domain.Quote, error) {
	s.getRandomCalls++
	return s.random, s.randomErr
}

func (s *stubQuoteRepository) GetByID(_ context.Context, id string) (*domain.Quote, error) {
	s.getByIDCalls++
	if s.byIDErr != nil {
		return nil, s.byIDErr
	}
	return s.byID[id], nil
}

func (s *stubQuoteRepository) List(_ context.Context, skip, take int) (domain.QuotePage, error) {
	s.listCalls = append(s.listCalls, stubListCall{skip: skip, take: take})
	return s.page, s.listErr
}

func (s *stubQuoteRepository) Add(_ context.Context, quote *domain.Quote) (domain.QuoteAddOutcome, error) {
	s.addCalls = append(s.addCalls, quote)
	return s.outcome, s.addErr
}

// sampleQuote rebuilds the catalog's canonical sample without validation.
func sampleQuote(t *testing.T) *domain.Quote {
	t.Helper()
	quote, err := domain.ReconstituteQuote(
		"7",
		"Programs must be written for people to read.",
		"Harold Abelson",
		"programs must be written for people to read")
	require.NoError(t, err)
	return quote
}

// requireCode asserts the error is a canonical domain error with the exact code.
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var domainErr *domain.Error
	require.ErrorAs(t, err, &domainErr)
	require.Equal(t, code, domainErr.Code)
}
