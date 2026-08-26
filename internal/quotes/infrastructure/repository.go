// Package infrastructure holds the quotes bounded context's adapters. This
// file is the PostgreSQL adapter for the domain.QuoteRepository port — the
// only code in the context that knows pgx exists (ADR 0007). Query semantics
// port the .NET PostgresQuoteRepository statement by statement: the random
// pick happens inside PostgreSQL, listing follows the stable seed-first order,
// and duplicate detection leans on the unique normalized_fingerprint index
// instead of a check-then-insert race.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/infrastructure/db"
)

// uniqueViolation is PostgreSQL's SQLSTATE 23505 (unique_constraint_violation):
// a colliding insert is the duplicate-detection signal, never a crash.
const uniqueViolation = "23505"

// PostgresQuoteRepository is the persistence adapter over the sqlc-generated
// queries; it is stateless beyond the shared pool, like the scoped .NET
// repository over its DbContext.
type PostgresQuoteRepository struct {
	queries *db.Queries
}

var _ domain.QuoteRepository = (*PostgresQuoteRepository)(nil)

// NewPostgresQuoteRepository wires the adapter over a pgx pool (any DBTX —
// pool or transaction — would do; the pool is the composed shape).
func NewPostgresQuoteRepository(pool *pgxpool.Pool) *PostgresQuoteRepository {
	return &PostgresQuoteRepository{queries: db.New(pool)}
}

// GetRandom returns a random quote, or domain.NotFound() when the catalog is
// empty.
func (r *PostgresQuoteRepository) GetRandom(ctx context.Context) (*domain.Quote, error) {
	row, err := r.queries.GetRandomQuote(ctx)
	return findOne(row, err)
}

// GetByID returns the quote with the given id, or domain.NotFound() when it
// does not exist.
func (r *PostgresQuoteRepository) GetByID(ctx context.Context, id string) (*domain.Quote, error) {
	if strings.TrimSpace(id) == "" {
		// A blank id is not one the catalog can hold; answering not-found
		// beats shipping a doomed round-trip (the .NET adapter rejects it
		// outright).
		return nil, domain.NotFound()
	}

	row, err := r.queries.GetQuoteById(ctx, id)
	return findOne(row, err)
}

// List returns the take items starting at offset skip in stable catalog order
// (created_at_utc, then id), with the total item count. Offsets beyond the
// end return an empty page, never an error.
func (r *PostgresQuoteRepository) List(ctx context.Context, skip, take int) (domain.QuotePage, error) {
	total, err := r.queries.CountQuotes(ctx)
	if err != nil {
		return domain.QuotePage{}, fmt.Errorf("counting quotes: %w", err)
	}

	// G115: both are bounded before they reach the adapter —
	// ListQuotesUseCase rejects a page size outside 1..domain.MaxPageSize and
	// an offset above math.MaxInt32, so neither conversion can wrap.
	rows, err := r.queries.ListQuotes(ctx, db.ListQuotesParams{
		Limit:  int32(take), //nolint:gosec // bounded by domain.MaxPageSize
		Offset: int32(skip), //nolint:gosec // bounded by math.MaxInt32 in the use case
	})
	if err != nil {
		return domain.QuotePage{}, fmt.Errorf("listing quotes: %w", err)
	}

	items := make([]*domain.Quote, 0, len(rows))
	for _, row := range rows {
		quote, err := QuoteRecord(row).toDomain()
		if err != nil {
			return domain.QuotePage{}, fmt.Errorf("reconstituting a listed quote: %w", err)
		}
		items = append(items, quote)
	}

	return domain.QuotePage{Items: items, Total: int(total)}, nil
}

// Add persists a quote atomically and reports whether it was written or
// rejected as a duplicate fingerprint. The insert either wins or fails with
// SQLSTATE 23505 from the unique fingerprint index (or, for a broken caller,
// the primary key) — both mean the quote already exists.
func (r *PostgresQuoteRepository) Add(ctx context.Context, quote *domain.Quote) (domain.QuoteAddOutcome, error) {
	if quote == nil {
		return domain.QuoteAddUnknown, errors.New("quote must not be nil")
	}

	err := r.queries.InsertQuote(ctx, db.InsertQuoteParams(newQuoteRecord(quote, time.Now().UTC())))
	if err == nil {
		return domain.QuoteAdded, nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.QuoteDuplicateFingerprint, nil
	}

	return domain.QuoteAddUnknown, fmt.Errorf("inserting quote %q: %w", quote.ID, err)
}

// QuoteRecord is one row of the quotes table: the persistence shape of a
// domain.Quote. It converts to and from the sqlc-generated row and parameter
// types with plain struct conversions — the shapes are identical by design
// (same fields, same order, same types), and the compiler rejects the
// conversions the day a regenerated shape drifts.
type QuoteRecord struct {
	ID                    string
	Text                  string
	Author                string
	NormalizedFingerprint string
	CreatedAtUtc          time.Time
}

// newQuoteRecord flattens a domain quote into a row, stamping the creation
// time — the .NET adapter stamps DateTimeOffset.UtcNow at SaveChanges.
func newQuoteRecord(quote *domain.Quote, createdAtUtc time.Time) QuoteRecord {
	return QuoteRecord{
		ID:                    quote.ID,
		Text:                  quote.Text.Value,
		Author:                quote.Author.Value,
		NormalizedFingerprint: quote.Fingerprint.Value,
		CreatedAtUtc:          createdAtUtc,
	}
}

// toDomain reconstitutes the domain aggregate. Persistence values are trusted
// (they passed create validation on the way in), so reconstitution only fails
// loudly on a mis-wired read — never on catalog data.
func (r QuoteRecord) toDomain() (*domain.Quote, error) {
	return domain.ReconstituteQuote(r.ID, r.Text, r.Author, r.NormalizedFingerprint)
}

// findOne converts a single-row fetch into the port's not-found contract:
// pgx.ErrNoRows means the catalog simply does not hold the quote, which is a
// domain outcome rather than an infrastructure failure, so it crosses the port
// as domain.NotFound() and every other error propagates as itself.
func findOne(row db.Quote, err error) (*domain.Quote, error) {
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.NotFound()
		}
		return nil, err
	}
	return QuoteRecord(row).toDomain()
}

// NewPool opens a pgx connection pool over databaseURL with sane defaults on
// top of pgxpool's own (which already sizes MaxConns to the machine):
// connections recycle before server-side lifetime and idle limits bite.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database url: %w", err)
	}

	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("opening the database pool: %w", err)
	}

	return pool, nil
}
