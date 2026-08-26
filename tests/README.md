# tests

The outer tiers of the testing pyramid (ADR 0008). The inner tiers — domain, application
and platform units, plus the testcontainers-backed repository suite — live next to the
code in `internal/**/*_test.go`; everything here crosses a process boundary.

| Tier | Lives in | Proves | Entry point |
|------|----------|--------|-------------|
| Wire tests | `cmd/authapi/main_test.go`, `cmd/quotesapi/wire_test.go` | the real composition root answering with the pinned wire semantics (error envelope, status table, paging, correlation echo, 401 challenge) | `make test` |
| BDD specs | [bdd](bdd/) | cross-service journeys in business language, through the real Traefik edge | `./scripts/bdd.sh` |
| Full-stack e2e | [e2e](e2e/) | the SPA from the `frontend/` submodule driven in Chromium against the real APIs + throwaway catalog | `./scripts/e2e.sh` |

The rule that keeps the suite from doubling (same as the .NET repo's): if it can be
proven without leaving one process, it does not belong in Gherkin. The exhaustive
validation permutations are domain unit tests; the specs get one scenario proving a
rejected quote surfaces as a 400 envelope to a caller who came through the edge.

## bdd/

godog (Gherkin) over the compose stack: `scripts/bdd.sh` brings up postgres + both APIs +
the edge under the `quotes-bdd` project (the [compose.bdd.yaml](bdd/compose.bdd.yaml)
overlay adds host ports for the docs/health surfaces and a spec-sized login rate limit),
runs `go test ./tests/bdd/...` with the `BDD_*` environment, and tears the project down.
26 scenarios across Authentication (signing in, introspection), Quotes (browsing,
publishing, authorization, the transcoded envelope) and Platform (health readiness, API
documentation). Step definitions are split by vocabulary (`steps_auth_test.go`,
`steps_quotes_test.go`, `steps_platform_test.go`, `steps_response_test.go`,
`steps_stack_test.go`); `world_test.go` holds the per-scenario state.

## e2e/

Just the Playwright configuration here — [playwright.config.ts](e2e/playwright.config.ts)
points the SPA's proxy at the two API processes `scripts/e2e.sh` starts, excludes the
scenarios that pin the v0/v1/v2 transports this backend does not serve, and runs the
feature files from `frontend/`. The suite boots with `workers: 1` because every scenario
shares the throwaway catalog.

How each tier gates in CI, and the coverage posture (collected, never gated):
[docs/testing.md](../docs/testing.md).
