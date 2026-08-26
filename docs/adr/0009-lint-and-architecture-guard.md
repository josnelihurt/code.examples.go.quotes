# ADR 0009 — Lint and architecture guard: golangci-lint v2 + depguard

* Status: accepted · Date: 2026-08-25*
* Related: guards the layers/packages decided in [ADR 0007](0007-persistence-sqlc-pgx-migrate.md) and [ADR 0008](0008-testing-strategy.md)

## Context

code.examples.net.quotes enforces quality with two gates: `dotnet format` (via
`scripts/lint.sh`, TreatWarningsAsErrors in Release) and NetArchTest.Rules layering tests
(`tests/Architecture.Tests/LayeringTests.cs`) turning project-reference drift into a red
test. The Go port wants one tool for both roles, run in CI exactly as locally.

## Decision

**golangci-lint v2.13.1** (current stable, verified 2026-08-25) with a committed
`.golangci.yml` in the v2 format (`version: "2"`; `linters:`; `formatters:`):

```yaml
version: "2"
linters:
  default: none
  enable: [errcheck, govet, staticcheck, unused, revive, misspell, copyloopvar, nolintlint, gocritic, depguard]
  settings:
    depguard:
      rules:
        quotes-domain:                        # one rule per layer per context
          files: ["internal/quotes/domain/**.go"]
          deny: [{pkg: ".../internal/quotes/application", desc: "domain imports nothing"}]  # + infra, api, auth, platform — table below
formatters:
  enable: [gofumpt]
```

The full rule set mirrors `LayeringTests`, across both bounded contexts:

| layer (files glob) | may import | denies |
| --- | --- | --- |
| `internal/{quotes,auth}/domain/**` | stdlib only | everything internal |
| `internal/{quotes,auth}/application/**` | stdlib + own domain | infra, api, other context, platform |
| `internal/{quotes,auth}/api/**` | stdlib + application + domain + platform | infrastructure, other context |
| `internal/{quotes,auth}/infrastructure/**` | stdlib + domain + application + platform | api, other context |
| `internal/platform/**` | stdlib only (leaf, ServiceDefaults analogue) | all contexts |
| `cmd/**` | everything (composition root) | — |

`internal/platform` is the ServiceDefaults analogue (config, middleware, JWT helpers) — a
leaf others may compose; application stays domain-only exactly as in the .NET table, which
additionally keeps the domain out of the api host (composition via DI) — a later tightening.

**Module layout (decided here): single module at the repo root** — one `go.mod` for
`github.com/josnelihurt/code.examples.go.quotes`, binaries under `cmd/<binary>`, code under
`internal/quotes|auth/{domain,application,infrastructure,api}` + `internal/platform`.
`internal/` keeps the layers un-importable outside the repo, one `go.mod` keeps the pins
in one place (the Directory.Packages.props analogue), depguard holds the arrows — no cycles.

**CI wiring.** `golangci/golangci-lint-action` pinned by commit SHA
(`ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a` = tag v9.3.0, resolved 2026-08-25) with
`version: v2.13.1`, so CI installs the same binary `brew install golangci-lint` provides
locally; the config's `version: "2"` pins the schema, and the `lint` job keeps the .NET
`ci.yml` path gating (the `dotnet format` gate's twin).

## Alternatives

- **goarchetype / custom `go/packages` test** — a Go port of NetArchTest; richer failure
  messages, but a second tool and a second CI job. depguard runs inside the existing lint
  gate, and package-level rules are what Go architecture rules should be — packages are
  the unit of coupling, as project references are in .NET.
- **`go vet`/Staticcheck CLI alone** — no aggregator; misses unused/errcheck/style drift.

## Consequences

- Layering drift fails `golangci-lint run` locally and the `lint` job in CI — review
  comments become red builds, same contract as `LayeringTests`.
- depguard rules glob on paths, not types; the per-context duplication (8 rules) is the
  price of explicitness and diffs well in review — a third context means four more rules.
- gofumpt (v0.11.0, via `formatters:`) is stricter than gofmt; `golangci-lint fmt` is
  the one-command formatter, and editors use the gofumpt binary.

## .NET mapping

| code.examples.net.quotes | Go port |
| --- | --- |
| `dotnet format` + analyzers (`scripts/lint.sh`) | `golangci-lint run` (errcheck, staticcheck, revive, …) |
| EditorConfig formatting rules | gofumpt via `formatters:` |
| NetArchTest.Rules `LayeringTests` | depguard rules (path-scoped deny lists) |
| Actions pinned by SHA | golangci-lint-action @ commit SHA, `version:` input |

## Pins

- golangci-lint **v2.13.1** (GitHub releases API + Homebrew, 2026-08-25)
- golangci-lint-action **v9.3.0** @ `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a`
- gofumpt **v0.11.0** (`mvdan.cc/gofumpt`, via `go list -m -versions`); gocritic, revive,
  misspell, copyloopvar and nolintlint bundle with golangci-lint
