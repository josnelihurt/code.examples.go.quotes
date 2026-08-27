# Architecture decisions

These decisions were evaluated in parallel research passes before the stack was cut; they pin
versions and map every .NET element to its Go replacement. Each ADR records the context, the
decision, the alternatives rejected, the .NET mapping and the exact version pins. The
[agentic workflow](agentic-workflow.md) describes how those research passes and the stack itself
were produced; [AGENTS.md](../AGENTS.md) carries the working agreements each layer follows.

| ADR | Title | Decision | Key pins |
| --- | --- | --- | --- |
| [0001](adr/0001-orchestration-compose-traefik.md) | Orchestration: compose spec + Traefik v3 edge | compose file is dev topology and publish artifact; Traefik file-provider routes replace YARP | traefik `v3.7.11`, postgres `18.6-alpine3.23`, pgweb `0.17.0`, nginx `1.31.4-alpine3.24`, node `24.19.0-alpine3.23` |
| [0002](adr/0002-v3-transport-grpc-gateway.md) | v3 transport: grpc-go server + grpc-gateway v2 runtime | stock gateway runtime serves the `google.api.http` annotations; every v3 wire knob mapped explicitly | grpc `v1.83.2`, protobuf `v1.36.12`, protoc-gen-go-grpc `v1.6.2`, grpc-gateway `v2.30.0` |
| [0003](adr/0003-openapi-generation-pipeline.md) | OpenAPI generation pipeline: hermetic, pinned to the .NET tool versions | buf + protoc-gen-openapiv2 at the .NET pins; frozen document, CI drift diff | buf `v1.50.0`, protoc-gen-openapiv2 `v2.30.0` |
| [0004](adr/0004-configuration-viper.md) | Configuration: Viper with typed structs and fail-fast validation | one Viper instance, `__` env-name parity with .NET, boot-time validation | viper `v1.21.0` |
| [0005](adr/0005-observability.md) | Observability: slog logging, OpenTelemetry traces and metrics | slog JSON + OTel matched core/contrib pair; telemetry on only when the OTLP endpoint is set | otel `v1.45.0`, otel-contrib `v0.70.0`, `log/slog` (stdlib) |
| [0006](adr/0006-auth-and-errors.md) | Auth, problem+json errors, rate limiting, routing, health | golang-jwt HS256 kit parity; hand-rolled problem+json envelope and fixed-window limiter; stdlib mux | golang-jwt/jwt/v5 `v5.3.1` |
| [0007](adr/0007-persistence-sqlc-pgx-migrate.md) | Persistence: sqlc + pgx/v5 + golang-migrate | SQL as the reviewed artifact, thin adapter over generated queries, boot migrations, verbatim seed | sqlc `v1.31.1`, pgx/v5 `v5.10.0`, golang-migrate `v4.19.1` |
| [0008](adr/0008-testing-strategy.md) | Testing strategy | testify unit, testcontainers integration, godog BDD through the Traefik edge, one `go test ./...` | testify `v1.12.1`, testcontainers-go `v0.44.0`, godog `v0.16.0` |
| [0009](adr/0009-lint-and-architecture-guard.md) | Lint and architecture guard: golangci-lint v2 + depguard | one tool for format and layering; depguard rules mirror the NetArchTest table | golangci-lint `v2.13.1`, action `v9.3.0`, gofumpt `v0.11.0` |
| [0010](adr/0010-single-port-edge-routing.md) | Single-port edge routing for dev surfaces | docs, pgweb, and SPA join Traefik by path prefix; one host port | amends ADR 0001 |
