# code.examples.go.quotes — repository rules

The load-bearing rules of *this* repository. Generic Go practice lives in
[go-rubric.md](go-rubric.md); this file is what makes a finding here specific and citable.
Verify a rule against the file named before reporting on it — this document summarizes, the
named file decides.

---

## R1. The layering guard is machine-enforced

`.golangci.yml` encodes a `depguard` rule per layer per bounded context — nine rules, so the
layering table fails lint rather than review (ADR 0009). The shape:

- **`quotes-domain`, `auth-domain`** — import *nothing* from their own upper layers
  (application, infrastructure, api), nothing from the other bounded context, and nothing from
  `internal/platform`. The domains are **stdlib-only**.
- **`quotes-application`, `auth-application`** — may not import their own `infrastructure`.
- **`quotes-api`, `auth-api`, `quotes-infrastructure`, `auth-infrastructure`** — carry their own
  deny lists; read the file.
- **`platform`** — a leaf the contexts compose; it may not depend on either bounded context.

A change that needs a new cross-layer import is either wrong or an architecture decision that
belongs in an ADR — never a `//nolint`.

**The two bounded contexts meet only at the composition root** (`cmd/quotesapi/`), through
ports defined by the consumer: `tokenValidator` (`wire.go`) and `bearerAuthenticator`
(`auth_adapter.go`) are the pattern.

## R2. The domain owns the error vocabulary, and the codes are public API

`internal/quotes/domain/errors.go` and `internal/auth/domain/errors.go` mint `*domain.Error`
values with a stable `Code`. Those codes surface on the wire as `errorCode`, so **renaming one is
a breaking change**. Transport maps code → status; the domain knows nothing about transport
(`statusFor` in `internal/auth/api/authapi.go`, `toStatusError` in
`internal/quotes/api/v3/service.go`).

## R3. The v3 wire semantics are pinned, not approximate

The wire tests and the specs pin them here, and a sibling .NET service over the same contract
pins them on its side. The semantics a change must not alter:

- the error envelope — `{"code":N,"message":…}` with `"details":[]` **present**
- the gRPC → HTTP status mapping (`toStatusError`)
- paging defaults **1 / 20** and the bounds; `page=0` sent explicitly is rejected, unset defaults
- the `X-Correlation-Id` echo (`internal/platform/correlation`)
- the 401 problem+json body with `WWW-Authenticate`
- create answers **200 with no `Location` header** — the HTTP rules cannot express 201+Location
- `loginResponse` field order: `accessToken, correlationId, expiresIn, username`
- `validateResponse.username` is `*string` so an invalid token writes an explicit `null`

Where a layer changes one of these deliberately, the ADR recording that decision is updated in
the same layer.

## R4. The proto is the single contract of record

`contracts/quotes/v3/quotes_v3.proto` is the source; `internal/quotes/api/v3/contract/*.pb.go`
and `docs/openapi/quotes-v3.openapi.json` are build output (ADR 0002, ADR 0003). **No
hand-written routing exists** — every wire semantic maps to a grpc-gateway knob pinned in
`NewGatewayMux`. Regenerate with `make contracts-go` (Go stubs) and `./scripts/update-contracts.sh`
(the hermetic OpenAPI rebuild the contract-drift CI job diffs). Hand-editing generated files is
blocking.

Likewise `internal/quotes/infrastructure/db/*.sql.go` is `sqlc` output from
`internal/quotes/infrastructure/queries.sql` against the `migrations/` schema (ADR 0007) —
regenerate with `sqlc generate`, pinned at v1.31.1 in `sqlc.yaml`.

## R5. `docs/openapi/` is a cross-repository convention

`docs/openapi/<name>.openapi.{json,yaml}` is where **all three repositories** publish their
frozen contract document. `frontend/.claude/agents/contract-syncer.md` and the frontend's
`contract-sync.yml` workflow hardcode that path against `code.examples.net.quotes`. Before
proposing that anything move out of `docs/openapi/`, grep `frontend/` and weigh the cross-repo
cost — see go-rubric.md §1.3 and the "cross-repository conventions outrank layout purity"
section of the `go-reviewer` agent.

The same caution applies to `contracts/` (the frontend uses `contracts/` too) and to `tests/`
(the shared `ci/secrets-hygiene` action's allowlist names `tests/**`).

## R6. Conventions are enforced, and a violating PR cannot merge

Full reference: `docs/contributing.md`. Enforced by the `conventions` CI job and a ruleset on
`main`:

- **Branches** `(feature|hotfix|chore|docs|ci|fix)/kebab-case`, issue-number suffix encouraged
  (`feature/e2e-db-19`). `backup/…` is local-only and exempt because it is never pushed.
- **Commit subjects and PR titles** `type(scope)!: lowercase imperative summary`, ≤72 chars,
  types `feat fix docs style refactor perf test build ci chore revert`, no trailing period.
- Check locally: `./scripts/check-conventions.sh --branch <name> --title "<title>" --range <base>..HEAD`
  (a thin exec over the `ci/` submodule — needs `git submodule update --init ci`).

## R7. Stacked PRs, every level independently green

`AGENTS.md` § "Big changes land as stacked pull requests" is the recipe and it is not optional.
One commit per branch; each branch cut from the previous head; **verification at every
load-bearing level, not only the tip**; PR body = What · Stack · Review pointers · Evidence-at-
this-level; register with `gh stack link`; label **only the top layer** `merge-me`. Never merge
by hand, never rebase or force-push a mid-stack branch, never label several layers (they race).

## R8. CI runs only the jobs a change can affect

The `changes` job of `.github/workflows/ci.yml` gates every job on the paths a PR touches. **A PR
that adds a job or moves a load-bearing file extends the filters in the same PR** — currently
`migrations/**` (~line 71), `contracts/**` (~84), `tests/README.md` (~123), `tests/**` (~259). The `ci:full-build`
label forces the full matrix. Skips are at job level on purpose: skipped check runs still satisfy
branch protection and still let merge-me's ci-completion trigger fire, which a workflow-level
`paths:` filter would break.

## R9. Documentation claims are mechanically verified

`./scripts/verify-docs.sh` runs two checks with different scopes. **Links and heading anchors**
are checked across `README.md`, `docs/**/*.md` (ADRs included), `cmd/**/README.md`,
`internal/**/README.md`, `tests/README.md` and `contracts/*.md`. **Backticked repo paths, routes
and identifiers** are checked on a narrower set: the component readmes plus `docs/architecture.md`,
`data-storage.md`, `testing.md`, `local-dev.md`, `observability.md`, `api.md` and
`system-design.md` — the root `README.md`, `docs/README.md`, the ADRs and the narrative pages are
outside it, so a stale citation there survives the gate. Neither check scans `CLAUDE.md` or
`.claude/**`, so links here are on the author. A rename that leaves a stale path in a
reference-checked page fails the gate.
`--skip-mermaid` skips the diagram render (which needs network and pnpm).

## R10. House style

Comments explain *why*, in full sentences, above the declaration, and they are dense — this
repository documents the constraint that forced each decision, usually citing the ADR that
records it. A change that reads as if a different author wrote it is a review finding.
Match the surrounding density; do not strip it and do not pad it.

## Gates

| Gate | Command |
|---|---|
| build | `make build` |
| tests (race + coverage) | `make test` |
| database integration | `make test-db` (needs a container runtime; skips itself without one) |
| lint | `make lint` |
| specs | `./scripts/bdd.sh` |
| full-stack e2e | `./scripts/e2e.sh` |
| contracts | `./scripts/update-contracts.sh` then confirm no drift |
| docs | `./scripts/verify-docs.sh --skip-mermaid` |
| conventions | `./scripts/check-conventions.sh --branch … --title … --range …` |
