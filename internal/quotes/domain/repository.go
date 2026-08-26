package domain

import "context"

// QuoteAddOutcome is the outcome of an atomic add. The adapter owns duplicate
// detection so callers never race between an existence check and an insert (in
// a database adapter this maps to catching the unique-index violation).
type QuoteAddOutcome int

const (
	// QuoteAdded means the quote was persisted.
	QuoteAdded QuoteAddOutcome = iota
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
	// GetRandom returns a random quote, or nil when the catalog is empty.
	GetRandom(ctx context.Context) (*Quote, error)

	// GetByID returns the quote with the given id, or nil when it does not
	// exist.
	GetByID(ctx context.Context, id string) (*Quote, error)

	// List returns the take items starting at offset skip in stable catalog
	// order, with the total item count. Offsets beyond the end return an empty
	// page, never an error.
	List(ctx context.Context, skip, take int) (QuotePage, error)

	// Add persists a quote atomically and reports whether it was written or
	// rejected as a duplicate fingerprint.
	Add(ctx context.Context, quote *Quote) (QuoteAddOutcome, error)
}
