# Go Quotes

**Go port of the Aspire Quotes backend**, serving the **v3 quotes transport**: the
[`quotes.v3` proto contract](contracts/quotes/v3/quotes_v3.proto) — identical messages and
`google.api.http` rules to the .NET original — driven through a grpc-go server behind the
grpc-gateway runtime, so the HTTP routes, the paging defaults and the gRPC status error
envelope all come from the contract, not from hand-written routing.

Stack: **Go 1.27**, **grpc-gateway v2**, **PostgreSQL** (sqlc + pgx + golang-migrate),
**compose + Traefik v3** as the edge, **godog** BDD and **Playwright** e2e.

## The repository family

| Repository | Role |
|------------|------|
| [code.examples.net.quotes](https://github.com/josnelihurt/code.examples.net.quotes) | the .NET original this port replicates (v3 transport, wire semantics, seed catalog) |
| [code.examples.frontend.quotes](https://github.com/josnelihurt/code.examples.frontend.quotes) | the React SPA, consumed here as a **git submodule pinned by commit** (`frontend/`); clone with `--recurse-submodules` |
| [code.examples.ci](https://github.com/josnelihurt/code.examples.ci) | the shared toolkit: `conventions`, `secrets-hygiene` and `merge-me` actions this repo's CI composes |

The deliverable is the same as the .NET seed's, restated for Go: a cloneable service shape
— clean architecture per bounded context (domain / application / infrastructure / api), a
shared platform kit, contracts as product — with every cross-cutting decision recorded as
an [ADR](docs/architecture-decisions.md) and mapped element-by-element from the .NET
original (the consolidated table lives in [docs/system-design.md](docs/system-design.md)).

## What runs

```text
Browser -> edge (Traefik :8080) --/api/v1/auth--> authapi   (login + token introspection, JWT HS256)
                                 --/api/v3/quotes-> quotesapi (v3 proto transport via grpc-gateway)
quotesapi validates tokens locally (golang-jwt); no per-request call to authapi
quotesapi -> postgres (migrated + seeded at boot: 8 quotes, unique fingerprint index)
```

- **authapi** — `POST /api/v1/auth/login` mints an HS256 JWT for the two documented local
  users (`jrb` holds `quotes:read` + `quotes:write`, `reader` holds `quotes:read`); both
  auth endpoints are rate-limited per client IP (fixed window). `POST /api/v1/auth/validate`
  is RFC 7662-style introspection. Errors are RFC 9457 problem+json.
- **quotesapi** — the v3 surface behind bearer authentication: `GET /api/v3/quotes/random`,
  `GET /api/v3/quotes/{id}`, `GET /api/v3/quotes?page=&page_size=` (1-based, defaults
  1/20, bounds 10000/100) and `POST /api/v3/quotes` (needs `quotes:write`; rejects invalid
  and near-duplicate quotes). Errors are the gRPC status envelope `{"code","message","details"}`;
  create answers 200 with no `Location` — the transcoding drift this transport exists to
  show. `/openapi/v3.json` serves the frozen, generated document and `/scalar` the
  reference UI.
- **postgres** — the catalog, volume-less on purpose: every boot migrates and seeds from
  scratch, the deterministic property the BDD and e2e suites assert on.

The full component map — with the per-layer readmes — is
[docs/system-design.md](docs/system-design.md); the transport rules and the layering
guard live in [docs/architecture.md](docs/architecture.md).

## How to run

```bash
./scripts/start.sh               # dev profile: postgres + both APIs + edge + docs + pgweb
```

Then log in through the edge with the documented development credentials
(`QUOTES_DEV_USERNAME` / `QUOTES_DEV_PASSWORD` from
[docs/dev-credentials.md](docs/dev-credentials.md)) — the script itself performs exactly
that round-trip to prove the stack: login, then one authenticated page of quotes.

| Service | URL | Profile |
|---------|-----|---------|
| edge (Traefik) | `http://localhost:8080` | every profile — the only published host port |
| Docsify docs + Scalar | `http://localhost:8080/docs/` | dev |
| pgweb (catalog browser) | `http://localhost:8080/pgweb/` | dev |
| Vite dev server (SPA) | `http://localhost:8080/` | fullstack |

```bash
./scripts/start.sh --core         # postgres + both APIs + edge only
./scripts/start.sh --fullstack    # dev + the SPA at /
./scripts/start.sh down           # tear it all down
```

The SPA's in-UI version switcher defaults to `v1`; pick `v3` there to drive this
backend's routes. Prerequisites and the day-to-day tasks:
[docs/local-dev.md](docs/local-dev.md).

## The suites

```bash
make test                        # every unit/integration suite: go test ./... -race (+coverage)
make test-db                     # the database integration suite against a local container runtime
./scripts/bdd.sh                 # godog specs against the real compose stack (26 scenarios)
./scripts/e2e.sh                 # full-stack Playwright: the SPA from the submodule, real APIs + catalog
./scripts/update-contracts.sh    # regenerate the frozen OpenAPI document after a proto change
```

`make test` covers the pyramid's fast tiers: domain invariants, application use cases,
transport wire tests, platform kit units — plus the testcontainers-backed repository
suite (`make test-db` runs it when no container runtime is on `DOCKER_HOST`). The BDD
suite boots the actual compose topology (postgres + both APIs + the Traefik edge) and
proves the cross-service journeys; the e2e suite drives the real UI in Chromium. Coverage
is collected and trended in CI, never gated. Details: [docs/testing.md](docs/testing.md).

## Documentation

- [Architecture](docs/architecture.md) — the compose + Traefik topology and the layering rules
- [Data storage](docs/data-storage.md) — sqlc + pgx + golang-migrate, the fingerprint rule, seeding
- [Testing](docs/testing.md) — the pyramid and how each tier gates in CI
- [Local dev](docs/local-dev.md) — prerequisites, profiles, common tasks
- [Observability](docs/observability.md) — slog, OTel, the `quotes.*` counters
- [API](docs/api.md) — the v3 surface, auth, error envelopes, `/openapi/v3.json` + `/scalar`
- [System design](docs/system-design.md) — end-to-end view incl. the .NET→Go mapping table
- [Architecture decisions](docs/architecture-decisions.md) — the nine ADRs the stack implements
- [Development credentials](docs/dev-credentials.md) — the single source of truth for non-Production secrets

The whole documentation set is mechanically verified: `./scripts/verify-docs.sh` checks
that every markdown link resolves and every backticked path, route and identifier the
pages cite exists in the code. CI runs it as the `docs` job on every docs-touching PR.

## How this repo is built

Not one long-lived coding session: a stack of pull requests — one decision per layer,
every level independently green — produced by three cooperating agent roles
(orchestrator, implementer, revisor). [docs/agentic-workflow.md](docs/agentic-workflow.md)
documents the loop; [AGENTS.md](AGENTS.md) carries the working agreements, and
[docs/contributing.md](docs/contributing.md) the branch/commit rules both humans and
agents follow, enforced by the ungated `conventions` job on every PR.

Large changes still land as stacked PRs, merged bottom-up by the `merge-me` label on the
top layer — see [docs/contributing.md](docs/contributing.md#enforcement) for how the
ruleset on `main` and the [merge-me workflow](.github/workflows/merge-me.yml) interact.
