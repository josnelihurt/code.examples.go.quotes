# Go Quotes

A production-shaped Go backend built as a statement of clean architecture: two bounded
contexts under an enforced layering table, a contract that owns its own transport, and a
decision record behind every cross-cutting choice. What it serves is the **v3 quotes
transport** — the proto contract driven by `google.api.http` annotations through
grpc-gateway, with the OpenAPI document generated from the same proto.

The repository README carries the full story (what runs, the local run, the suites); this
site is the documentation set:

- [Architecture](architecture.md) — the compose + Traefik topology, the layering rules, the transport rule
- [Data storage](data-storage.md) — sqlc + pgx + golang-migrate, the unique-fingerprint rule, seeding
- [Testing](testing.md) — the pyramid and how each tier gates in CI
- [Local dev](local-dev.md) — prerequisites, profiles, common tasks
- [Observability](observability.md) — slog, OTel, the `quotes.*` counters, correlation
- [API](api.md) — the v3 surface, auth, error envelopes, the reference pages
- [System design](system-design.md) — end-to-end view: topology, request lifecycle, CI, technology choices
- [Architecture decisions](architecture-decisions.md) — the ten ADRs the stack implements
- [Contributing](contributing.md) — branch naming, commit subjects, enforcement
- [Agentic workflow](agentic-workflow.md) — how coding agents work in this repository
- [Development credentials](dev-credentials.md) — the single source of truth for non-Production secrets
- [Scalar (v3 API)](scalar/) — the interactive reference

## How this repo is built

Each layer of the stack was specified, implemented and reviewed by a different
fresh-context agent role — orchestrator, implementer, revisor — looping implement→revise
until the revisor passed; the [agentic workflow](agentic-workflow.md) page documents the
loop, the revisor checklist and the merge handoff, and [architecture
decisions](architecture-decisions.md) records the evaluations the layers implement.

The set is mechanically verified: `./scripts/verify-docs.sh` checks that every link
resolves, and that the pages carrying code citations — the component readmes and the
seven reference pages here — cite only paths, routes and identifiers that exist in the
code. The ADRs and the narrative pages are link-checked only. The `docs` CI job runs the
same gate on every docs-touching PR.
