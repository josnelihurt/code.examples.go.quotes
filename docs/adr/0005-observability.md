# ADR 0005 — Observability: slog logging, OpenTelemetry traces and metrics

* Status: accepted · Date: 2026-08-25*

## Context

The .NET kit (`src/ServiceDefaults/Extensions.cs`, `SerilogExtensions.cs`, `Telemetry/AppMetrics.cs`)
wires Serilog (console sink, `UseSerilogRequestLogging`, `CorrelationId` pushed into `LogContext`) and
OpenTelemetry (ASP.NET Core/HttpClient/runtime instrumentation; a custom meter `AspireQuotesPoc` with
counters `quotes.random.count`, `quotes.getbyid.count`, `quotes.list.count`, `quotes.create.count` —
plus `auth.login.count`, `auth.validate.count` — each incremented with an `outcome` tag drawn from the
ErrorOr vocabulary success|not_found|error|invalid|conflict). The OTLP exporter installs only when
`OTEL_EXPORTER_OTLP_ENDPOINT` is set; `/health` and `/alive` are excluded from tracing; the correlation
middleware stamps both an Activity tag and a baggage member `correlation.id`.

## Decision

1. **Logging: `log/slog` (stdlib, Go 1.27).** One `*slog.Logger` built in the composition root —
   `slog.NewJSONHandler` to stdout with `service.name`/environment attributes replacing Serilog's
   enrichers; level Info by default with a settable `slog.LevelVar` for per-area overrides (the
   `Microsoft.AspNetCore`->Warning pattern). A `RequestLogger` middleware replaces
   `UseSerilogRequestLogging`: one Info line per request with method, path, status, duration and
   correlationId, attached via `logger.With(...)` — the Go analogue of `LogContext.PushProperty`.
2. **Traces and metrics: OpenTelemetry Go, matched pair.** Core v1.45.0 with contrib v0.70.0 —
   contrib v0.70.0's `go.mod` pins core v1.45.0 (verified on proxy.golang.org; core v1.46.0 was tagged
   2026-08-25 without a matching contrib release, so it is noted, not pinned). Exporters:
   `otlptracegrpc` and `otlpmetricgrpc` with a periodic metric reader.
3. **Enabled-when-endpoint-set parity / graceful degradation:** providers are installed only when
   `OTEL_EXPORTER_OTLP_ENDPOINT` is non-empty; otherwise the globals remain the SDK's no-op
   implementations and every instrumentation call is a near-free no-op — the exact behavior of
   `AddOpenTelemetryExporters()`. No other env parsing; the OTel SDK reads its own protocol/header vars.
4. **HTTP instrumentation:** wrap the router with `otelhttp.NewHandler` (server spans) and dial outbound
   clients through `otelhttp.Transport`; `otelhttp.WithFilter` skips `/health` and `/alive`, parity with
   the ASP.NET Core tracing filter. `otelgrpc` stays pinned but unused until a gRPC surface exists.
5. **Custom metrics:** `meter := otel.Meter("code.examples.go.quotes")` (counterpart of meter name
   `AspireQuotesPoc`); one `meter.Int64Counter("quotes.random.count", metric.WithDescription(...))` per
   use case; increments via `counter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", o)))`
   with `o` in success|not_found|error|conflict|invalid — the `UseCaseTelemetry.Outcome` vocabulary
   ported verbatim as one shared helper used by all decorators.
6. **Correlation across services:** the middleware ([ADR 0006](0006-auth-and-errors.md)) sets the span attribute `correlation.id`
   (`span.SetAttributes`) and attaches baggage (`baggage.NewMember("correlation.id", id)` +
   `context.WithBaggage`), so the next hop receives it in the W3C `baggage` header via the global
   propagator; the request logger carries the same value on every line.

## Alternatives

- **zap / zerolog** — faster allocation profiles, but slog is stdlib, structured and handler-composable;
  logging throughput is not a kit constraint and no third-party logger lands on the request path.
- **Serilog's OTLP log sink** — not re-implemented: Go's OTel logs SDK is still maturing, and
  stdout-to-collector covers the PoC topology; revisit if a log-ingest requirement appears.
- **Prometheus `client_golang`** — pull model with different naming; the reference exports OTLP.

## Consequences

- No collector endpoint: zero telemetry, zero exporter goroutines. One env var turns the full pipeline
  on — deployment parity with Aspire's `UseOtlpExporter()`.
- Counter names and `outcome` values are public metric contract; tests pin them (as `AppMetricsTests` does).
- Core and contrib are bumped together as a pair; the pin waits for contrib v0.71.0 before taking
  core v1.46.0.
- JSON logs to stdout only — operators pipe to collectors; no file sink.

## .NET mapping

| .NET (reference repo)                              | Go (this ADR)                                  |
|----------------------------------------------------|------------------------------------------------|
| Serilog console + `UseSerilogRequestLogging`       | slog JSON handler + RequestLogger middleware   |
| `LogContext.PushProperty("CorrelationId")`         | `logger.With("correlationId", id)`             |
| `AddOpenTelemetry().WithTracing(AddAspNetCore...)` | `sdk/trace` TracerProvider + `otelhttp`        |
| `WithMetrics().AddMeter("AspireQuotesPoc")`        | `sdk/metric` MeterProvider + `otel.Meter`      |
| `Counter<long>.Add(1, outcome tag)`                | `Int64Counter.Add(ctx, 1, WithAttributes)`     |
| `Activity.SetTag` + `AddBaggage("correlation.id")` | span attribute + `baggage` member + propagator |
| `UseOtlpExporter()` when endpoint set              | install providers only when endpoint set       |
| health/alive excluded from tracing                 | `otelhttp.WithFilter`                          |

## Pins

- `go.opentelemetry.io/otel` **v1.45.0** (core; v1.46.0 tagged 2026-08-25, awaiting matching contrib)
- `go.opentelemetry.io/otel/trace`, `otel/metric`, `otel/sdk`, `otel/sdk/metric` **v1.45.0**
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` **v1.45.0**
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` **v1.45.0**
- `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` **v0.70.0** (2026-08-04)
- `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc` **v0.70.0** (reserve)
- `log/slog` — stdlib, Go 1.27.0
