# Architecture

This page states the **rules** of the port: what runs where, how the layers may depend
on each other, and how the wire contracts are owned. The end-to-end map — including CI —
is [system design](system-design.md); the decisions behind each rule are the
[ADRs](architecture-decisions.md); each component's detail lives in the readme next to
its source.

> Links from this Docsify site into repository paths (`../cmd/...`, `../internal/...`)
> resolve on GitHub; Docsify serves the `docs/` folder only.

## Topology

The compose file is both the dev stack and the publish artifact (ADR 0001) — the .NET
original's Aspire AppHost expressed in the engine-neutral compose spec, so the same file
runs under docker compose (CI) and podman compose (laptops). The edge is Traefik v3 with
the file provider: no docker labels, no engine socket, one watched route table
(`traefik/dynamic.yml`).

```mermaid
flowchart LR
  user["Browser"]
  edge["edge - Traefik v3 :8080"]
  auth["authapi"]
  quotes["quotesapi"]
  pg[("postgres - quotesdb")]
  docs["docs - nginx /docs"]
  pgweb["pgweb /pgweb"]
  vite["frontend - Vite /app"]
  dash["Traefik dashboard /"]

  user --> edge
  edge -->|"/api/v1/auth"| auth
  edge -->|"/api/v3/quotes"| quotes
  edge -->|"/docs"| docs
  edge -->|"/pgweb"| pgweb
  edge -->|"/app"| vite
  edge -->|"/ → /dashboard/"| dash
  quotes -->|migrate + seed at boot| pg
  pgweb --- pg
```

Traefik is the only published host port. API routes keep `PathPrefix` semantics — the
APIs see the request path exactly as the SPA emits it. Dev surfaces join by path prefix
(ADR 0010): StripPrefix keeps docs and pgweb serving from `/` internally, while the SPA
carries no middleware and is built under its own prefix instead (`VITE_BASE_PATH`):

| Route | Service | Notes |
|-------|---------|-------|
| `/api/v1/auth` | authapi | login + introspection |
| `/api/v3/quotes` | quotesapi | the proto transport |
| `/docs` | docs | Docsify + Scalar (dev profile) |
| `/pgweb` | pgweb | catalog browser (dev profile) |
| `/app` | frontend | Vite SPA (fullstack profile) |
| `/` → `/dashboard/` | Traefik | API dashboard (every profile) |

`authapi` owns no database; `quotesapi` never calls authapi on the request path — it
validates tokens locally with the shared HS256 key (`JWT__SIGNINGKEY`, one value for both
sides; see [dev credentials](dev-credentials.md)). Healthchecks + `depends_on` replace
Aspire's `WaitFor`, and the quotesapi boot additionally retries database connects itself
because podman-compose's conditional ordering has bug history.

Profiles: unprofiled = core (postgres + both APIs + edge); `dev` adds docs and pgweb
behind `/docs` and `/pgweb`; `fullstack` adds the Vite dev server at `/app`. When the SPA
is up, browser API calls are same-origin through the edge. `scripts/start.sh` drives the
selection.

## Layering

Two bounded contexts (quotes, auth) plus a platform kit, each context with the same four
layers — the .NET Clean Architecture shape. Dependencies point one way only, and the
rules are code, not aspiration: the depguard section of `.golangci.yml` (ADR 0009) fails
lint on any violation, mirroring the .NET repo's NetArchTest table.

```mermaid
flowchart TD
  subgraph composition ["composition roots (cmd/)"]
    root["authapi / quotesapi main"]
  end
  subgraph quotes ["quotes context"]
    qapi["api/v3"]
    qapp["application"]
    qdom["domain"]
    qinfra["infrastructure"]
  end
  subgraph auth ["auth context"]
    aapi["api"]
    aapp["application"]
    adom["domain"]
    ainfra["infrastructure"]
  end
  subgraph platform ["platform kit"]
    plat["config, correlation, httpserver, problemjson, telemetry"]
  end

  root --> quotes
  root --> auth
  root --> plat
  qapi --> qapp
  qapi --> qdom
  qapi --> plat
  qapp --> qdom
  qinfra --> qdom
  aapi --> aapp
  aapi --> adom
  aapi --> plat
  aapp --> adom
  ainfra --> adom
  ainfra --> plat
```

The composition roots are drawn against the whole context because that is what they
touch: `cmd/` wires every layer of both contexts, which is the point of a composition
root and the reason the layering rules below say nothing about it.

The rules that matter:

1. **Domain imports nothing** — not upper layers, not the other context, not the
   platform kit. Both domains are in fact stdlib-only, but that last step is convention:
   depguard names module-internal packages only, so third-party imports are invisible to
   it and a domain package staying stdlib-only is held by review.
2. **Application** depends on its own domain; never infrastructure, api, the other
   context or the platform kit (the platform is composed at the edges).
3. **Infrastructure** implements the domain's ports; it may never reach up into api. It
   may read the platform kit — auth's adapters take their settings from `config` — since
   the kit is a leaf below everything.
4. **The api layer never imports its own infrastructure**; it receives the adapters it
   needs from the composition root.
5. **Bounded contexts meet only in composition** — the guard denies the other context
   from api, application and infrastructure alike, which leaves `cmd/` as the sanctioned
   meeting point: quotesapi's `bearerAuthenticator` (in `cmd/quotesapi`) adapts the auth
   context's validator onto the v3 transport's `Authenticator` port. No depguard rule
   covers `cmd/`; what keeps the seam honest is that only the composition roots import
   both contexts.
6. **Platform is a leaf** others compose; it may not depend on either context.

Per-layer ownership and the readmes: [internal/quotes](../internal/quotes/README.md),
[internal/auth](../internal/auth/README.md), [internal/platform](../internal/platform/README.md).

## The transport rule (v3)

The proto file is the single contract of record: every route comes from its
`google.api.http` rules via the grpc-gateway runtime — **no hand-written routing exists**
in this transport. The gateway knobs that pin the wire semantics (the JSONPb marshaler
that keeps `"details":[]` in every error body, the correlation header matchers, the
stock error handler that produces the `{"code","message","details"}` envelope and the
gRPC→HTTP status table) are enumerated in [ADR 0002](adr/0002-v3-transport-grpc-gateway.md).
Three of them are set in one place, `NewGatewayMux`: the marshaler and the two
correlation header matchers. The fourth is deliberately *not* set — the runtime's
`DefaultHTTPErrorHandler` is left untouched, so the envelope and the status table are the
stock ones rather than a local reimplementation of them.

Authentication and authorization run **before** the gateway (the `RequireScope`
middleware): the 401 problem+json with `WWW-Authenticate` and the empty-body 403 are
answered there, byte-identical to the .NET pipeline, and never surface through the
gateway's error handler. A grpc-side interceptor enforces the same method→scope table
for any caller that dials the loopback listener directly — defense in depth, not the
wire path.

## Configuration and errors

Configuration is Viper with typed structs and `__` env-name parity with the .NET kit
(`JWT__SIGNINGKEY`, `CONNECTIONSTRINGS__QUOTESDB`, `SERVER__ADDRESS`), validated
fail-fast at boot (ADR 0004). Expected failures are `*domain.Error` values with stable
codes; the two transports map them differently on purpose — problem+json on auth (and on
the quotes transport's pre-gateway 401/403), the gRPC status envelope on the v3 surface
(ADR 0006). Both envelopes are single-sourced: `problemjson` for problems, the gateway's
error handler for statuses.

## Observability and data

Logging is slog JSON, telemetry is OpenTelemetry on only when
`OTEL_EXPORTER_OTLP_ENDPOINT` is set (ADR 0005) — [observability](observability.md).
The catalog is PostgreSQL behind sqlc + pgx + golang-migrate with migration-at-boot
(ADR 0007) — [data storage](data-storage.md).

## Layout deviations

The repository departs from `golang-standards/project-layout` in three places. That
document is a community convention rather than a Go team standard, and each departure
below was reviewed and kept deliberately. They are recorded here so a reader — or a
review agent — does not have to re-derive the reasoning.

**`tests/` rather than `test/`.** The convention names the directory `test/`. Renaming it
would reach `scripts/bdd.sh`, `scripts/e2e.sh`, `scripts/verify-docs.sh`, two path
filters in `.github/workflows/ci.yml`, and the allowlist inside the shared
[code.examples.ci](https://github.com/josnelihurt/code.examples.ci) submodule — a second
repository. The rename buys a convention and costs a cross-repository change, so the
directory stays.

**`contracts/` rather than `api/`.** Also deliberate, and for a stronger reason:
`contracts/` is the name the sibling repositories use for the same concept, so the
consistency that matters here is across the three repositories rather than with the
layout document.

**Layer package names repeat across bounded contexts.** `domain`, `application`,
`infrastructure` and `api` each exist twice — once under `internal/quotes/` and once
under `internal/auth/` — so composition roots import two packages with one name and must
alias one (`authinfra` in `cmd/quotesapi/main.go`). That is the price of the
bounded-context layout the depguard table enforces, and it is worth paying: the
alternative is prefixing every package with its context and reading
`quotesdomain.Quote` everywhere the layering already makes obvious.

A fourth case — a Go package living under `docs/` — is recorded where the decision
belongs, in [ADR 0003](adr/0003-openapi-generation-pipeline.md).
