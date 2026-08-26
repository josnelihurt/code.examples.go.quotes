# platform

The shared host kit every API composes — the .NET seed's `ServiceDefaults` project,
expressed as a leaf Go package. Depguard pins it: platform may not import either
bounded context; the contexts compose it at their edges.

| Package | Owns |
|---------|------|
| [config](config/) | Viper loading with typed structs, `__`-separated env parity (`JWT__SIGNINGKEY`) and the fail-fast `Validate` (ADR 0004) |
| [correlation](correlation/) | the `X-Correlation-Id` middleware: accept-or-mint, echo, context + span + baggage (ADR 0006) |
| [httpserver](httpserver/) | `/health` + `/alive` handlers, the slog request logger, the graceful-shutdown serve loop |
| [problemjson](problemjson/) | the single RFC 9457 envelope (`type`/`title`/`detail`/`status` + `errorCode`, `correlationId`, `traceId`) every problem body renders through |
| [telemetry](telemetry/) | the OTel pipeline — no-op without `OTEL_EXPORTER_OTLP_ENDPOINT` — plus the six custom counters (ADR 0005) |

Each package carries its own test file (`config_test.go`, `correlation_test.go`, …)
inside the `make test` sweep. The observability page unpacks the counter names and
outcome vocabularies: [docs/observability.md](../../docs/observability.md).
