# ADR 0007 — Persistence: sqlc + pgx/v5 + golang-migrate

* Status: accepted · Date: 2026-08-25*

## Context

code.examples.net.quotes persists a single `quotes` table through EF Core
(`QuotesDbContext` on Npgsql). The port `IQuoteRepository` lives in the domain;
`PostgresQuoteRepository` is a thin adapter whose duplicate detection leans on the unique
`NormalizedFingerprint` index — a colliding insert surfaces as Postgres error `23505` and
maps to `QuoteAddOutcome.DuplicateFingerprint`, never a check-then-insert race. Migrations
run at boot (`Database.MigrateAsync()` in `Program.cs`, idempotent, EF 9's database-wide
lock serializing replicas); `HasData` seeds 8 rows (ids 1–8, `2024-01-01T00:00:00Z`,
precomputed fingerprints). Decided upstream: sqlc + pgx + golang-migrate + testcontainers.

## Decision

**sqlc v1.31.1 with the built-in Go codegen (`gen: go`, `sql_package: "pgx/v5"`).** Queries
live as plain SQL under `internal/quotes/infrastructure/persistence/queries/`; `sqlc
generate` output is committed next to them. The domain port keeps the exact .NET shape
(`GetRandom`/`GetByID` returning nil when absent, `List(skip, take)` returning the page,
`Add` returning `Added` or `DuplicateFingerprint`); a thin adapter in infrastructure — the
only code that knows pgx exists — implements it over the generated `*Queries` (built over
`*pgxpool.Pool`, sqlc's `DBTX`):

- `GetRandom`: `SELECT id, text, author, created_at_utc FROM quotes ORDER BY random()
  LIMIT 1` — exact parity with the EF `FromSql` raw query (random pick inside PostgreSQL).
- `List`: `ORDER BY created_at_utc, id` with `LIMIT/OFFSET` plus a separate `COUNT(*)` —
  the same stable order (seeds first, id tiebreaker) and page contract.
- `Add`: plain `INSERT`; `var pgErr *pgconn.PgError; errors.As(err, &pgErr) &&
  pgErr.Code == "23505"` returns `DuplicateFingerprint`; every other error propagates.

**golang-migrate v4.19.1**, migrations embedded (`//go:embed *.sql` from
`internal/quotes/infrastructure/migrations` +
`source/iofs`, `database/pgx` driver), applied by the API host before serving — boot
parity with `MigrateAsync`, advisory lock serializing replicas; no migrate CLI in the image.

**Migration 0001 mirrors the .NET `20260823175731_InitialCreate`** — seed values verbatim:

```sql
CREATE TABLE quotes (
    id varchar(64) PRIMARY KEY, text varchar(280) NOT NULL, author varchar(80) NOT NULL,
    normalized_fingerprint varchar(280) NOT NULL, created_at_utc timestamptz NOT NULL);
CREATE UNIQUE INDEX quotes_normalized_fingerprint_key ON quotes (normalized_fingerprint);
INSERT INTO quotes (id, text, author, normalized_fingerprint, created_at_utc) VALUES
 ('1','Simplicity is the ultimate sophistication.','Leonardo da Vinci','simplicity is the ultimate sophistication','2024-01-01T00:00:00Z'),
 ('2','Code is like humor. When you have to explain it, it''s bad.','Cory House','code is like humor when you have to explain it it s bad','2024-01-01T00:00:00Z'),
 ('3','First, solve the problem. Then, write the code.','John Johnson','first solve the problem then write the code','2024-01-01T00:00:00Z'),
 ('4','Experience is the name everyone gives to their mistakes.','Oscar Wilde','experience is the name everyone gives to their mistakes','2024-01-01T00:00:00Z'),
 ('5','The only way to go fast is to go well.','Robert C. Martin','the only way to go fast is to go well','2024-01-01T00:00:00Z'),
 ('6','Make it work, make it right, make it fast.','Kent Beck','make it work make it right make it fast','2024-01-01T00:00:00Z'),
 ('7','Programs must be written for people to read.','Harold Abelson','programs must be written for people to read','2024-01-01T00:00:00Z'),
 ('8','Talk is cheap. Show me the code.','Linus Torvalds','talk is cheap show me the code','2024-01-01T00:00:00Z');
```

Columns are deliberately snake_case where EF emitted quoted PascalCase
(`"NormalizedFingerprint"`): both sides speak hand-written SQL, no EF model must stay
compatible, and the public contract is HTTP, not the database's identifier casing (so the
two services cannot share a catalog without aliases — they never do).

## Alternatives

- **GORM** — runtime-reflection ORM; duplicates EF's opacity (SQL inferred, not reviewed).
- **ent** — schema-codegen graph ORM; heavyweight for a single-table catalog, and its
  generated entities would leak into the domain.
- **database/sql** — no dependencies but hand-written `rows.Scan` boilerplate with no
  compile-time check that SQL and Go types agree; sqlc gives that check, no runtime.

## Consequences

- SQL is reviewed, not inferred — the query text is the artifact, like the EF raw query;
  `sqlc generate` output is committed so CI builds without the tool.
- `ORDER BY random()` is a full sort per pick — fine at PoC catalog size (the trade the
  .NET repo documents); swapping to `TABLESAMPLE` later touches one query.
- The 23505 mapping depends on pgx's typed error; the adapter owns `errors.As`.

## .NET mapping

| code.examples.net.quotes | Go port |
| --- | --- |
| EF Core `QuotesDbContext` mapping | sqlc schema + generated models (infrastructure only) |
| Npgsql provider | `github.com/jackc/pgx/v5` (`pgxpool`, `pgconn.PgError`) |
| `PostgresQuoteRepository` | sqlc `*Queries` wrapped by a thin adapter |
| `DbUpdateException`→`PostgresException` 23505 | `errors.As(*pgconn.PgError)` code 23505 |
| `Database.MigrateAsync()` at boot | golang-migrate `iofs` Up at boot (advisory lock) |
| `HasData` seed / `InitialCreate` | migration 0001 DDL + verbatim INSERT rows |

## Pins

Verified 2026-08-25 via `go list -m -versions` (Go module proxy): sqlc **v1.31.1**
(built-in Go codegen; no sqlc-gen-go plugin needed) · pgx/v5 **v5.10.0** · golang-migrate **v4.19.1**.
