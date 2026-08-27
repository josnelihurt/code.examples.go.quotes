# quotes context

The quotes bounded context, layered exactly like the .NET original's Quotes projects —
one folder per layer, dependencies pointing one way only, enforced by depguard (see
[ADR 0009](../../docs/adr/0009-lint-and-architecture-guard.md)):

| Layer | Folder | Owns | May import |
|-------|--------|------|------------|
| Domain | [domain](domain/) | the `Quote` aggregate, value objects (`Text`, `Author`, `Fingerprint`), canonical error codes, paging rules, the `QuoteRepository` port | the standard library only |
| Application | [application](application/) | the four use cases (`GetRandomQuoteUseCase`, `GetQuoteByIDUseCase`, `ListQuotesUseCase`, `CreateQuoteUseCase`) and their DTOs | domain |
| Infrastructure | [infrastructure](infrastructure/) | the PostgreSQL repository over sqlc-generated queries, boot migration, the health round-trip | domain, application |
| API (v3) | [api/v3](api/v3/) | the grpc-gateway transport: gateway mux, the QuoteService implementation, the auth middleware, the doc endpoints | domain, application, platform |

Rules the guard pins (`.golangci.yml` depguard section): domain imports nothing from its
upper layers, other contexts or the platform; application may not reach into
infrastructure or api; the api layer composes infrastructure only through the composition
root; and the two bounded contexts meet nowhere under `internal/` — api, application and
infrastructure each deny the other context, so `cmd/` is the only place they meet.

Failures are `*domain.Error` values carrying stable, wire-visible codes
(`quote.not_found`, `quote.duplicate_fingerprint`, `quote.invalid_page_request`, …) —
renaming one is a breaking change; the transport maps them to statuses and the codes
surface in error envelopes.

The near-duplicate rule is a database constraint, not a discipline: the fingerprint the
domain computes is stored under a unique index, and the repository's `Add` reports the
duplicate outcome from the insert itself (see
[data storage](../../docs/data-storage.md)). What each route does with these outcomes:
[docs/api.md](../../docs/api.md).
