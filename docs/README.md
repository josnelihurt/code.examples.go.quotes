# Go Quotes

Go port of the [code.examples.net.quotes](https://github.com/josnelihurt/code.examples.net.quotes)
backend, serving the **v3 quotes transport** — the proto contract driven by `google.api.http`
annotations through grpc-gateway, with the OpenAPI document generated from the same proto.

The platform lands as a stack of pull requests tracked in the issues; see the
[repository README](../README.md) for intention and conventions.

## How this repo is built

Each layer of that stack is specified, implemented and reviewed by a different fresh-context
agent role — orchestrator, implementer, revisor — looping implement→revise until the revisor
passes; the [agentic workflow](agentic-workflow.md) page documents the loop, the revisor
checklist and the merge handoff, and [architecture decisions](architecture-decisions.md)
records the evaluations the layers implement.

## Quick links

- [Contributing](contributing.md) — branch naming, commit subjects, enforcement
- [Architecture decisions](architecture-decisions.md) — the port's recorded decisions
- [Agentic workflow](agentic-workflow.md) — how coding agents work in this repository
