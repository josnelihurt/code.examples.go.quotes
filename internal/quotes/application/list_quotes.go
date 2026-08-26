package application

import (
	"context"
	"math"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
)

// ListQuotesQuery is a 1-based page request. Validation happens in the use
// case; the defaults live in the domain's QuoteRules constants.
type ListQuotesQuery struct {
	Page     int
	PageSize int
}

// ListQuotesUseCase returns one page of the catalog in stable order.
type ListQuotesUseCase struct {
	quotes domain.QuoteRepository
}

// NewListQuotesUseCase assembles the use case on top of the repository port.
func NewListQuotesUseCase(quotes domain.QuoteRepository) *ListQuotesUseCase {
	return &ListQuotesUseCase{quotes: quotes}
}

// Execute returns the requested page with its navigation arithmetic, or
// quote.invalid_page_request when the page or page size is outside the allowed
// range. A page beyond the end of the catalog returns empty items, never an
// error.
func (uc *ListQuotesUseCase) Execute(ctx context.Context, query ListQuotesQuery) (QuotePageDto, error) {
	if err := ctx.Err(); err != nil {
		return QuotePageDto{}, err
	}

	if query.Page < 1 || query.Page > domain.MaxPage ||
		query.PageSize < 1 || query.PageSize > domain.MaxPageSize {
		return QuotePageDto{}, domain.InvalidPageRequest()
	}

	// The rules above bound the product well below math.MaxInt32; the int64
	// arithmetic is defense in depth so a future rule change fails closed
	// instead of wrapping the skip.
	skip := int64(query.Page-1) * int64(query.PageSize)
	if skip > math.MaxInt32 {
		return QuotePageDto{}, domain.InvalidPageRequest()
	}

	page, err := uc.quotes.List(ctx, int(skip), query.PageSize)
	if err != nil {
		return QuotePageDto{}, err
	}

	items := make([]QuoteDto, 0, len(page.Items))
	for _, quote := range page.Items {
		items = append(items, toDto(quote))
	}

	return QuotePageDto{
		Items:      items,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalItems: page.Total,
		TotalPages: int((int64(page.Total) + int64(query.PageSize) - 1) / int64(query.PageSize)),
	}, nil
}
