# System design

A single view of the whole system: what runs, how a request travels from the browser to
the catalog and back, and how the repository is built and shipped. This page is the
**map**; the **rules** live in [architecture](architecture.md) and the per-component
detail in a `README.md` next to each source tree.

> The component links below are repository paths (`../cmd/...`, `../internal/...`).
> They resolve on GitHub; in the Docsify site they 404 (Docsify serves `docs/` only).

## System context

```mermaid
flowchart LR
  user["Browser"]
  edge["edge - Traefik v3 :8080"]
  auth["authapi"]
  quotes["quotesapi"]
  pg[("postgres - quotesdb")]
  site["docs - nginx /docs"]
  spa["frontend - Vite SPA /app"]
  dash["Traefik dashboard /"]

  user --> edge
  edge --> auth
  edge --> quotes
  edge --> site
  edge --> spa
  edge --> dash
  quotes --> pg
```

Two bounded contexts, one edge, one database, one SPA. Traefik is the only published
host port; `/` opens the Traefik dashboard, and dev surfaces plus the SPA join by path
prefix (ADR 0010). `authapi` mints tokens; `quotesapi` validates them locally
(golang-jwt) and never calls back on the request path. The SPA is consumed as a pinned
git submodule — its own repository carries its tests and its lint gates; from this
checkout it runs in the fullstack profile and the e2e suite.

## Runtime topology

```mermaid
flowchart LR
  key["AUTH_SIGNING_KEY - one shared HS256 value"]
  pg["postgres - pinned image, volume-less"]
  auth["authapi - Go image, :8080 in-container"]
  quotes["quotesapi - Go image, :8080 in-container"]
  edge["edge - Traefik, host :8080 only"]
  docs["docs - nginx /docs (dev profile)"]
  pgweb["pgweb /pgweb (dev profile)"]
  vite["frontend - Vite /app (fullstack profile)"]
  dash["dashboard / → /dashboard/"]

  key -->|"JWT__SIGNINGKEY"| auth
  key -->|"JWT__SIGNINGKEY"| quotes
  quotes -->|"CONNECTIONSTRINGS__QUOTESDB + migrate/seed at boot"| pg
  pgweb --- pg
  edge --> auth
  edge --> quotes
  edge --> docs
  edge --> pgweb
  edge --> vite
  edge --> dash
```

Three things carry the wiring:

- **One secret, two services.** The compose default of `AUTH_SIGNING_KEY` is a public
  local-development value (see [dev credentials](dev-credentials.md)); tokens minted by
  authapi must validate in quotesapi, so both receive it as `JWT__SIGNINGKEY` — Viper's
  `__` separator mapping onto `jwt.signingkey`.
- **The catalog is volume-less on purpose.** quotesapi migrates and seeds at boot
  ([data storage](data-storage.md)), so every stack up is the deterministic eight-quote
  catalog the specs assert on.
- **Healthchecks order the boot** (`depends_on: condition: service_healthy`), and the
  quotesapi boot additionally retries database connects itself for a bounded budget —
  the podman-compose caveat recorded in ADR 0001.

## Request lifecycle

One authenticated read, end to end:

```mermaid
sequenceDiagram
  participant U as Browser/SPA
  participant E as edge (Traefik)
  participant A as authapi
  participant Q as quotesapi
  participant P as postgres

  U->>E: POST /api/v1/auth/login {username, password}
  E->>A: PathPrefix(/api/v1/auth)
  A-->>U: 200 {accessToken, expiresIn, correlationId}
  U->>E: GET /api/v3/quotes?page=1 (Authorization: Bearer, X-Correlation-Id)
  E->>Q: PathPrefix(/api/v3/quotes)
  Q->>Q: RequireScope middleware - validate JWT, check quotes:read
  Q->>Q: gateway mux -> loopback grpc -> v3 service -> use case
  Q->>P: sqlc-generated list query
  P-->>Q: one page + total
  Q-->>U: 200 {items, page, pageSize, totalItems, totalPages}
```

Rejections happen at different layers by design: the 401/403 shapes before the gateway,
domain rejections (unknown id, invalid page, duplicate fingerprint) inside the service
as gRPC statuses that the gateway renders as `{"code","message","details"}` — the exact
table is on [API](api.md).

## CI

`.github/workflows/ci.yml` gates every job (except `conventions`, `secrets-hygiene` and
path detection itself) on the areas a PR touches — the `changes` job computes the
filters, the jobs consume the `run_*` outputs. Skips happen at the job level on purpose:
skipped check runs still satisfy branch protection, and the workflow still completes so
[merge-me](../.github/workflows/merge-me.yml) keeps firing. Pushes to `main`, the
`ci:full-build` label and CI-configuration edits force the full matrix.

| Job (check name) | Proves | Runs on |
|------------------|--------|---------|
| `conventions (branch names + commit messages)` | branch, commits, PR title rules | every PR and push, ungated |
| `secrets hygiene (credential literals stay in the allowlist)` | credential literals consolidated | every PR and push, ungated |
| `openapi contract drift` | the frozen document matches a hermetic rebuild | `contracts/**`, `docs/openapi/**`, `Dockerfile.build` |
| `container image pin drift` | compose/Dockerfiles quote `scripts/images.env` exactly | pin files + compose + API Dockerfiles |
| `docs (links + code references)` | every documented link/ref resolves | `docs/**`, the readmes, `contracts/*.md` |
| `build & test (race + coverage)` | the whole unit/wire/integration sweep, race on | backend changes, pins |
| `lint (golangci-lint)` | format + the depguard layering rules | backend changes |
| `security (CodeQL)` | static analysis of the Go module | backend changes |
| `specs (BDD against the compose stack)` | the 26 Gherkin journeys through the edge | backend changes, pins |
| `e2e (full-stack Playwright against the Go APIs)` | the SPA against real APIs + catalog | backend changes, frontend pointer, pins |

## Technology choices

One row per cross-cutting choice, with the reason it was made; the ADR beside it carries
the alternatives that were rejected and the exact version pins.

| Choice | Why | Where |
|--------|-----|-------|
| the compose spec as both dev topology and publish artifact | one engine-neutral file, so CI (docker compose) and laptops (podman compose) run the same topology instead of two drifting descriptions | [ADR 0001](adr/0001-orchestration-compose-traefik.md), `docker-compose.yaml` |
| Traefik v3 with the file provider | the route table is a reviewed file watched live — no container labels scattered across services, no engine socket handed to the edge | [ADR 0001](adr/0001-orchestration-compose-traefik.md), `traefik/dynamic.yml` |
| sqlc + pgx/v5 + golang-migrate, migration at boot | SQL stays the artifact under review and the generated access code stays typed; every boot converges the schema itself | [ADR 0007](adr/0007-persistence-sqlc-pgx-migrate.md) |
| grpc-go + the grpc-gateway v2 runtime over the proto | routes, paging defaults and the error envelope are derived from the contract, so there is no hand-written routing to drift from it | [ADR 0002](adr/0002-v3-transport-grpc-gateway.md) |
| `log/slog` JSON | structured logging from the standard library: nothing to pin, nothing to vendor, one line per request | [ADR 0005](adr/0005-observability.md) |
| otel-go with a matched core/contrib pair | traces and metrics behind a single endpoint check — with no OTLP endpoint set the pipeline is a no-op and costs nothing | [ADR 0005](adr/0005-observability.md) |
| golang-jwt/jwt/v5 validated in-process, plus `RequireScope` | no per-request call to the auth service; scope is enforced before the transport and again at the grpc boundary | [ADR 0006](adr/0006-auth-and-errors.md) |
| `internal/platform/problemjson` | one RFC 9457 envelope every problem body renders through, so the error shape cannot diverge per handler | [ADR 0006](adr/0006-auth-and-errors.md) |
| the `testing` package, table tests under `t.Run` | the toolchain's own runner, so a failing case names itself and needs no framework to read | [ADR 0008](adr/0008-testing-strategy.md) |
| testify assert/require | readable failure messages without inventing an assertion DSL | [ADR 0008](adr/0008-testing-strategy.md) |
| hand-written stubs (`stub_repository_test.go`, `stubs_test.go`) | the ports are small; a mocking framework would cost more to read than the stub costs to write | [ADR 0008](adr/0008-testing-strategy.md) |
| godog for the specs | the cross-service journeys stay in business language and run against the real edge | [ADR 0008](adr/0008-testing-strategy.md) |
| testcontainers-go for repository tests | the repository is tested against a real PostgreSQL, fresh per test, rather than a fake | [ADR 0008](adr/0008-testing-strategy.md) |
| `go test -coverprofile` | coverage is collected and trended, never gated — a threshold buys ceremony, not correctness | [ADR 0008](adr/0008-testing-strategy.md) |
| golangci-lint depguard rules | the layering table is machine-checked on every PR, which is what makes it a rule rather than a diagram | [ADR 0009](adr/0009-lint-and-architecture-guard.md) |
| a static Scalar page over the embedded frozen document | the reference UI ships with the service and serves the same bytes CI diffs for drift | [ADR 0002](adr/0002-v3-transport-grpc-gateway.md), `internal/quotes/api/v3/docs.go` |
| the `docs` compose service (nginx, dev profile, `/docs` on the edge) | the documentation site runs beside the stack it documents, on the same front door | ADR 0001, ADR 0010, `docker-compose.yaml` |
| Viper with typed structs and `__` env-name parity | one loader, validated fail-fast at boot; the `__` names carry unchanged to the sibling implementation of this contract | [ADR 0004](adr/0004-configuration-viper.md) |

## How this repository was built

The ten-pull-request stack this repository was originally built as — three cooperating
agent roles, one decision per layer, every level independently green — is documented in
[agentic workflow](agentic-workflow.md); the working agreements each layer followed are
[AGENTS.md](../AGENTS.md) and [contributing](contributing.md).
