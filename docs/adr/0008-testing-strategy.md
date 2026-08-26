# ADR 0008 — Testing strategy

* Status: accepted · Date: 2026-08-25*
* Related: [ADR 0007](0007-persistence-sqlc-pgx-migrate.md) — testcontainers fixtures for the repository contract

## Context

code.examples.net.quotes runs a five-layer pyramid: xunit.v3 unit tests with Shouldly
asserts and NSubstitute doubles; Testcontainers.PostgreSql integration tests (one container
per run, per-test database); Reqnroll BDD specs booting the Aspire stack via
Aspire.Hosting.Testing and speaking HTTP through the YARP gateway; Playwright
(+playwright-bdd) e2e in the frontend submodule; coverlet coverage collected and uploaded
but never gated. The Go port keeps the shape, not the libraries.

## Decision

1. **Unit — stdlib `testing` + testify v1.12.1 (`assert`/`require`).** Testify over
   stdlib-only: the Shouldly parity play — `require.Equal` fails fast with a message-rich
   diff, `assert.*` keeps collecting failures, matching `ShouldBe` ergonomics. Doubles are
   **hand-written stubs** — the ports are NSubstitute-sized narrow interfaces (4 methods on
   `QuoteRepository`), so a fake is one small struct with recorded calls; gomock's codegen
   and generated mocks would cost more than the stub and hide failures in generated code.
2. **Integration — testcontainers-go v0.44.0, `modules/postgres` v0.44.0.** One
   `postgres.Container` per test run, `CREATE DATABASE quotes_test_<uuid>` per test —
   direct parity with `PostgresTestDatabase.cs`. The image resolves from the repo's single
   pin (`scripts/images.env`, `POSTGRES_IMAGE` env override) so CI, BDD, e2e and compose
   all boot one image; DSN from `container.ConnectionString(ctx)`. Repository-contract
   tests (GetRandom/GetByID/List/Add + 23505) run against a fresh migrated database.
3. **BDD — cucumber/godog v0.16.0.** Feature files mirror the Reqnroll ones:
   TranscodedQuotes, Authorization, BrowsingQuotes, PublishingQuotes, HealthReadiness,
   ApiDocumentation (same Gherkin, same scenario names). The suite speaks HTTP through the
   **Traefik edge of the compose stack** — the Go analogue of speaking through the YARP
   gateway under Aspire.Hosting.Testing. CI keeps a dedicated `specs` job: `docker compose
   up -d`, wait for Traefik readiness, run the suite. godog runs **as a `go test` wrapper**
   (`godog.TestSuite` inside `TestMain`), keeping the toolchain `go test ./tests/bdd/...`;
   the suite skips itself unless `QUOTES_STACK=compose` is set, so `go test ./...` stays
   green on a Docker-less laptop.
4. **E2E — unchanged.** Playwright stays in code.examples.frontend.quotes; the fullstack
   config's `webServer` boots the Go binaries exactly as it booted the Release DLLs, and
   the `e2e` job keeps: build binaries, throwaway PostgreSQL from `scripts/e2e.env`,
   `pnpm run test:e2e:fullstack`.
5. **Wire tests — transport parity in-process.** `httptest.NewServer` over the assembled
   API mux with a locally minted JWT (same signing key and claims builder the auth module
   exports), asserting headers, bodies and error envelopes per transport version — the
   container-free descendant of the TranscodedWire/TransportParity suites.
6. **Coverage — `go test ./... -race -coverprofile=coverage.out -covermode=atomic`**,
   artifact uploaded from the build-and-test job, **no threshold gate** — parity with
   coverlet (collected and trended, never gating).

## Alternatives

- **stdlib-only asserts** — zero deps, but every failure message is hand-rolled; the .NET
  repo's assertion ergonomics would be lost, not ported.
- **gomock / go.uber.org/mock** — justified when interfaces are wide; here it adds a
  generator to CI and mock files nobody reads for interfaces a stub covers in 15 lines.
- **Ginkgo/Gomega or a godog-CLI CI step** — drift from `go test ./...` (no `-race`,
  no coverage reuse); the TestMain wrapper keeps both for free.

## Consequences

- One command (`go test ./...`) runs unit + wire + (with Docker) integration and BDD —
  fewer moving parts than dotnet's per-project invocations.
- The BDD job needs the compose stack; its readiness wait is the CI seam that fails
  honestly when Traefik or an upstream service is broken (the specs job's property).
- Traefik replaces YARP as the gateway under test, so specs pin Traefik's forwarding
  behavior (X-Correlation-Id echo, header casing) rather than YARP's.
- Coverage percentages are not comparable to coverlet's line coverage; trend within the
  Go repo only.

## .NET mapping

| code.examples.net.quotes | Go port |
| --- | --- |
| xunit.v3 3.2.2 | stdlib `testing` (+ `go test ./...`) |
| Shouldly 4.3.0 | testify `assert`/`require` |
| NSubstitute 6.0.0 | hand-written stubs over narrow ports |
| Testcontainers.PostgreSql 4.14.0 | testcontainers-go postgres module |
| Reqnroll 3.3.4 features | godog feature files (same Gherkin) |
| Aspire.Hosting.Testing + YARP | compose stack + Traefik edge over HTTP |
| Playwright e2e (frontend submodule) | unchanged — same repo, same job |
| coverlet.collector 10.0.1 | `-coverprofile` + CI artifact, no gate |

## Pins

Verified 2026-08-25 via `go list -m -versions` (Go module proxy):

- `github.com/stretchr/testify` **v1.12.1**
- `github.com/testcontainers/testcontainers-go` **v0.44.0**
- `github.com/testcontainers/testcontainers-go/modules/postgres` **v0.44.0**
- `github.com/cucumber/godog` **v0.16.0**
