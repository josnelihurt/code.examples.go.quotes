-- The catalog's reviewed SQL (ADR 0007): the query text is the artifact, the
-- .NET adapter's EF/raw-query semantics ported one statement at a time.

-- name: GetRandomQuote :one
-- Random pick happens inside PostgreSQL — exact parity with the EF FromSql
-- raw query. The catalog is PoC-sized, so a full sort per pick is the simple,
-- correct tool. An empty catalog surfaces as pgx.ErrNoRows in the adapter.
SELECT id, text, author, normalized_fingerprint, created_at_utc
FROM quotes
ORDER BY random()
LIMIT 1;

-- name: GetQuoteById :one
SELECT id, text, author, normalized_fingerprint, created_at_utc
FROM quotes
WHERE id = $1;

-- name: ListQuotes :many
-- Stable catalog order: seeds first (they share a fixed timestamp), then
-- created quotes in creation order, with the id as a deterministic tiebreaker.
SELECT id, text, author, normalized_fingerprint, created_at_utc
FROM quotes
ORDER BY created_at_utc, id
LIMIT $1 OFFSET $2;

-- name: CountQuotes :one
SELECT COUNT(*) FROM quotes;

-- name: InsertQuote :exec
-- Plain INSERT on purpose: duplicate detection leans on the unique
-- normalized_fingerprint index instead of a check-then-insert race — a
-- colliding insert fails with PostgreSQL error 23505, which the adapter maps
-- to the QuoteDuplicateFingerprint outcome.
INSERT INTO quotes (id, text, author, normalized_fingerprint, created_at_utc)
VALUES ($1, $2, $3, $4, $5);
