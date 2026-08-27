# platform

The shared host kit every API composes, as a leaf below both bounded contexts. Depguard
pins that: platform may not import either context; the contexts compose it at their
edges.

| Package | Owns |
|---------|------|
| [config](config/) | Viper loading with typed structs, `__`-separated env parity (`JWT__SIGNINGKEY`) and the fail-fast `Validate` (ADR 0004) |
| [correlation](correlation/) | the `X-Correlation-Id` middleware: accept-or-mint, echo, context + span + baggage (ADR 0006) |
| [httpserver](httpserver/) | `/health` + `/alive` handlers, the slog request logger, the graceful-shutdown serve loop |
| [problemjson](problemjson/) | the single RFC 9457 envelope (`type`/`title`/`detail`/`status` + `errorCode`, `correlationId`, `traceId`) every problem body renders through |
| [telemetry](telemetry/) | the OTel pipeline — no-op without `OTEL_EXPORTER_OTLP_ENDPOINT` — plus the six custom counters (ADR 0005) |

The kit is a leaf, not a flat set: `telemetry` composes `httpserver` (for the probe-path
filter), and `httpserver` and `problemjson` both compose `correlation`. Depguard says
nothing about imports *inside* `internal/platform` — the ordering is held by review, and
a new cycle here would be a design change, not a lint failure.

Each package carries its own test file (`config_test.go`, `correlation_test.go`, …)
inside the `make test` sweep. The observability page unpacks the counter names and
outcome vocabularies: [docs/observability.md](../../docs/observability.md).
