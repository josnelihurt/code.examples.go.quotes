package application

import (
	"context"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
)

// CreateQuoteCommand is the raw input for publishing a new quote.
type CreateQuoteCommand struct {
	Text   string
	Author string
}

// CreateQuoteUseCase validates and publishes a new quote into the catalog.
type CreateQuoteUseCase struct {
	quotes domain.QuoteRepository
}

// NewCreateQuoteUseCase assembles the use case on top of the repository port.
func NewCreateQuoteUseCase(quotes domain.QuoteRepository) *CreateQuoteUseCase {
	return &CreateQuoteUseCase{quotes: quotes}
}

// Execute creates the quote and persists it atomically. Invalid input returns
// the validation error from the domain; a duplicate fingerprint returns
// quote.duplicate_fingerprint and nothing is written.
func (uc *CreateQuoteUseCase) Execute(ctx context.Context, command CreateQuoteCommand) (QuoteDto, error) {
	if err := ctx.Err(); err != nil {
		return QuoteDto{}, err
	}

	quote, err := domain.NewQuote(command.Text, command.Author)
	if err != nil {
		return QuoteDto{}, err
	}

	outcome, err := uc.quotes.Add(ctx, quote)
	if err != nil {
		return QuoteDto{}, err
	}

	if outcome == domain.QuoteDuplicateFingerprint {
		return QuoteDto{}, domain.DuplicateFingerprint()
	}

	return toDto(quote), nil
}
