package domain

import "context"

// QuoteAddOutcome is the outcome of an atomic add. The adapter owns duplicate
// detection so callers never race between an existence check and an insert (in
// a database adapter this maps to catching the unique-index violation).
type QuoteAddOutcome int

const (
	// QuoteAddUnknown is the zero value: no outcome was determined. It is what
	// an adapter returns beside a non-nil error, so a caller that reads the
	// outcome without checking the error first gets a value that means
	// nothing rather than one that reads as success.
	QuoteAddUnknown QuoteAddOutcome = iota
	// QuoteAdded means the quote was persisted.
	QuoteAdded
	// QuoteDuplicateFingerprint means the catalog already holds a quote with
	// the same fingerprint and nothing was written.
	QuoteDuplicateFingerprint
)

// QuotePage is one page of the catalog in adapter order, plus the total item
// count so callers can compute page counts without a second query.
type QuotePage struct {
	Items []*Quote
	Total int
}

// QuoteRepository is the persistence port for the quote catalog.
type QuoteRepository interface {
	// GetRandom returns a random quote, or an error satisfying
	// errors.Is(err, NotFound()) when the catalog is empty. The returned quote
	// is non-nil whenever the error is nil.
	GetRandom(ctx context.Context) (*Quote, error)

	// GetByID returns the quote with the given id, or an error satisfying
	// errors.Is(err, NotFound()) when it does not exist — a blank id included,
	// since no catalog can hold one. The returned quote is non-nil whenever
	// the error is nil.
	GetByID(ctx context.Context, id string) (*Quote, error)

	// List returns the take items starting at offset skip in stable catalog
	// order, with the total item count. Offsets beyond the end return an empty
	// page, never an error.
	List(ctx context.Context, skip, take int) (QuotePage, error)

	// Add persists a quote atomically and reports whether it was written or
	// rejected as a duplicate fingerprint.
	Add(ctx context.Context, quote *Quote) (QuoteAddOutcome, error)
}
