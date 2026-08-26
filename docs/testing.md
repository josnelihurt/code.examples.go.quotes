# Testing

Four tiers, each owning exactly one question — the .NET repo's pyramid, restated for Go
(ADR 0008). The bottom two are exhaustive and fast; the top two are few scenarios in
business language, run against real processes.

| Tier | Lives in | Proves | Style |
|------|----------|--------|-------|
| Unit | `internal/**/*_test.go`, `cmd/**/*_test.go` | Domain invariants and error codes, use-case arithmetic, adapter contracts, platform kit behavior | exhaustive, microseconds, `testify` assertions over hand stubs |
| Wire tests | `cmd/authapi/main_test.go`, `cmd/quotesapi/wire_test.go` | The real composition root answering with the pinned wire semantics: the `{"code","message","details"}` envelope with `"details":[]`, the gRPC→HTTP status table, paging defaults (1/20) and bounds, the `X-Correlation-Id` echo, the 401 problem+json with `WWW-Authenticate`, create answering 200 with no `Location` | in-process, per endpoint |
| Specs | `tests/bdd` | Cross-service journeys through the real Traefik edge, in Gherkin | few (26 scenarios), godog |
| E2E | `tests/e2e` + `frontend/` | What a human does in a browser | fewest, Playwright + Chromium |

**The rule that keeps the suite from doubling:** if it can be proven without leaving
one process, it does not belong in Gherkin. Every `quote.text_*` / `quote.author_*`
validation permutation is a domain unit test; the specs carry one scenario proving a
rejected quote surfaces as a 400 envelope to a caller who came through the edge.

## Stack

| Area | Tools |
|------|-------|
| Units + wire | the `testing` package + `testify` (assertions/require), hand-written stubs |
| Database integration | testcontainers-go: one throwaway PostgreSQL container, a fresh database per test (`make test-db` on a laptop; automatic in CI) |
| Specs | godog (Gherkin) against the compose stack |
| E2E | Playwright + playwright-bdd, features from the frontend submodule |

## Running each tier

```bash
make test          # units + wire: go test ./... -race, coverage collected
make test-db       # the repository suite against a local container runtime
./scripts/bdd.sh   # specs: compose stack up, suite, teardown (--no-teardown keeps it up)
./scripts/e2e.sh   # full stack: real APIs + throwaway catalog + the SPA in Chromium
```

The database integration suite skips itself when no container runtime is reachable on
`DOCKER_HOST` — `make test-db` points `DOCKER_HOST` at the podman machine socket when
one is running. `scripts/e2e.sh` requires `E2E_SIGNING_KEY`
([dev credentials](dev-credentials.md)); CI synthesizes an ephemeral one.

The specs' environment details — the `quotes-bdd` compose project, the overlay's host
ports and spec-sized login rate limit — are on [tests/README.md](../tests/README.md).

## How the tiers gate in CI

Every gate runs the same entry point a developer runs:

| CI job | Tier | Path filter that triggers it |
|--------|------|------------------------------|
| `build & test (race + coverage)` | units + wire + database integration | `**/*.go`, `go.mod`/`go.sum`, `sqlc.yaml`, `internal/quotes/infrastructure/migrations/**`, `.golangci.yml`, `Makefile`, image pins |
| `specs (BDD against the compose stack)` | specs | backend changes + image pins |
| `e2e (full-stack Playwright against the Go APIs)` | e2e | backend changes + the `frontend` submodule pointer + image pins |

A markdown-only change runs none of them; a docs change runs the `docs` gate instead —
the filters live in the `changes` job of `.github/workflows/ci.yml`, and a PR that adds
a gate extends the filters in the same PR. The `ci:full-build` label forces the full
matrix, as does every push to `main`.

## Coverage policy

Coverage is **collected and trended, never gated** (ADR 0008): CI runs
`go test ./... -race -coverprofile=coverage.out` and uploads the artifact, but no
threshold fails a build. The race detector is the gate that matters — the domain's
concurrency-free core should never have data races.

## What is covered

- **quotes domain** — value-object rules (`text.go`, `author.go`, `fingerprint.go`),
  aggregate composition incl. the author-equals-text rule, value-object equality
- **quotes application** — random (empty-catalog path), get-by-id, list paging
  arithmetic and range rejection, create success/invalid/conflict, all over a stubbed
  repository port
- **quotes infrastructure** — the repository contract against real PostgreSQL via
  testcontainers (list paging, no-overlap, beyond-end, the duplicate-fingerprint
  outcome over the seed catalog)
- **v3 transport** — the gateway wire tests above, plus the auth middleware's 401/403
  shapes and the scope table
- **auth** — login success/failure, blank input, JWT round-trip, expiry and key
  mismatch, scope claims, hardcoded credentials (incl. the Production refusal), rate
  limiter windows
- **platform** — config loading and the fail-fast validation, correlation echo/mint,
  problem+json shapes, telemetry counters and the no-op-without-endpoint path
- **specs** — signing in (good, wrong password, blank input), introspection, browsing
  (random, by-id 404 envelope, default paging, invalid page), publishing (success,
  invalid, duplicate, reader-scope 403), health readiness degrading while the catalog
  database is down, the OpenAPI/Scalar surfaces
- **e2e** — the SPA's sign-in, random-quote, catalog and publish journeys against the
  v3 transport; scenarios pinning v0/v1/v2 are excluded by name in
  `tests/e2e/playwright.config.ts`
