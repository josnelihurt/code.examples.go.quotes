package application

import (
	"context"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
)

// GetRandomQuoteUseCase returns one quote from the catalog at random.
type GetRandomQuoteUseCase struct {
	quotes domain.QuoteRepository
}

// NewGetRandomQuoteUseCase assembles the use case on top of the repository
// port.
func NewGetRandomQuoteUseCase(quotes domain.QuoteRepository) *GetRandomQuoteUseCase {
	return &GetRandomQuoteUseCase{quotes: quotes}
}

// Execute returns a random quote, or quote.not_found when the catalog is empty.
func (uc *GetRandomQuoteUseCase) Execute(ctx context.Context) (QuoteDto, error) {
	if err := ctx.Err(); err != nil {
		return QuoteDto{}, err
	}

	quote, err := uc.quotes.GetRandom(ctx)
	if err != nil {
		return QuoteDto{}, err
	}

	if quote == nil {
		return QuoteDto{}, domain.NotFound()
	}

	return toDto(quote), nil
}
