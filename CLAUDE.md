# CLAUDE.md

Entry point for coding agents. The working agreements live in
[AGENTS.md](AGENTS.md) — read it before changing anything.

## Read first

| Document | What it settles |
|---|---|
| [AGENTS.md](AGENTS.md) | Branch and commit conventions (enforced); the stacked-PR recipe; how CI gating works |
| [docs/agentic-workflow.md](docs/agentic-workflow.md) | The Orchestrator / Implementer / Revisor loop and the revisor checklist |
| [docs/contributing.md](docs/contributing.md) | The conventions reference in full |
| [docs/architecture.md](docs/architecture.md) | Bounded contexts, layering, and where things live |
| [.claude/skills/go-review/references/repo-rules.md](.claude/skills/go-review/references/repo-rules.md) | The load-bearing rules, summarized and cross-referenced |

## Gates

`make build` · `make test` · `make lint` — CI runs the same commands, so a green laptop means a
green build. Suite scripts: `./scripts/bdd.sh`, `./scripts/e2e.sh`,
`./scripts/verify-docs.sh --skip-mermaid`, `./scripts/check-conventions.sh`.

## Non-negotiables

- **Branches** `(feature|hotfix|chore|docs|ci|fix)/kebab-case`; **subjects and PR titles**
  `type: lowercase imperative summary`, ≤72 chars. A violating PR cannot merge.
- **Big changes land as stacked PRs**, one commit per branch, **every level independently
  green**. Never one large PR.
- **Never merge by hand** — label only the top layer `merge-me`.
- **Generated code is never hand-edited** — `*.pb.go`, `*.sql.go`, `docs/openapi/*.json`.
  Regenerate with `make contracts-go`, `sqlc generate`, `./scripts/update-contracts.sh`.
- **The domains are stdlib-only.** `depguard` in `.golangci.yml` enforces the layering table.

## Tooling

`/go-review` reviews the tree and produces ranked findings · `/triage-findings` turns findings
into issues you selected · `/stack-pr` lands issues as a verified stack. Agents:
`go-reviewer` (read-only), `stack-implementer`, `stack-revisor` (read-only).
