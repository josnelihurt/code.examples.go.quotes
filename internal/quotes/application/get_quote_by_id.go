package application

import (
	"context"
	"strings"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
)

// GetQuoteByIDUseCase returns a single quote by its catalog id.
type GetQuoteByIDUseCase struct {
	quotes domain.QuoteRepository
}

// NewGetQuoteByIDUseCase assembles the use case on top of the repository port.
func NewGetQuoteByIDUseCase(quotes domain.QuoteRepository) *GetQuoteByIDUseCase {
	return &GetQuoteByIDUseCase{quotes: quotes}
}

// Execute returns the quote with the given id, or quote.not_found — including
// for a blank id, which never reaches the repository.
func (uc *GetQuoteByIDUseCase) Execute(ctx context.Context, id string) (QuoteDto, error) {
	if err := ctx.Err(); err != nil {
		return QuoteDto{}, err
	}

	if strings.TrimSpace(id) == "" {
		return QuoteDto{}, domain.NotFound()
	}

	quote, err := uc.quotes.GetByID(ctx, id)
	if err != nil {
		return QuoteDto{}, err
	}

	if quote == nil {
		return QuoteDto{}, domain.NotFound()
	}

	return toDto(quote), nil
}
