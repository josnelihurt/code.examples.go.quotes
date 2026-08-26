// Package application implements the quotes use cases on top of the domain
// repository port. Use cases return (T, error) where a rejected request is a
// *domain.Error carrying the wire-visible code the transport maps to an HTTP
// status; infrastructure failures surface as raw errors for the transport to
// map to a 500.
package application

import "github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"

// QuoteDto is the transport-facing projection of a quote.
type QuoteDto struct {
	ID     string
	Text   string
	Author string
}

// QuotePageDto is one page of quotes with the arithmetic a client needs to
// build page navigation.
type QuotePageDto struct {
	Items      []QuoteDto
	Page       int
	PageSize   int
	TotalItems int
	TotalPages int
}

// toDto projects a quote aggregate into its transport shape.
func toDto(quote *domain.Quote) QuoteDto {
	return QuoteDto{
		ID:     quote.ID,
		Text:   quote.Text.Value,
		Author: quote.Author.Value,
	}
}
