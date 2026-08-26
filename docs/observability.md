# Observability

The port's telemetry stack ([ADR 0005](adr/0005-observability.md)): slog JSON logging,
OpenTelemetry traces and metrics, correlation — with no collector required to boot.

## The no-op-without-endpoint rule

`telemetry.Setup` installs OTLP gRPC exporters **only when
`OTEL_EXPORTER_OTLP_ENDPOINT` is set**. Without it the OTel globals stay the SDK's no-op
implementations — every instrumentation call is near-free, both APIs log one line saying
so, and nothing else changes. The compose topology deliberately sets no endpoint: local
observability is the container log stream, not a dashboard port (the .NET stack's Aspire
dashboard has no analogue here by design — see the ADR's rejected alternatives).

Point the variable at any OTLP gRPC receiver to light the pipeline up; the W3C trace
context + baggage propagators are registered either way.

## Logs (slog)

Every API writes slog JSON to stdout with `service.name` and `environment` pre-attached,
one request line per request from the platform request logger (method, path, status,
duration), and the correlation id attached — filter a stream by
`X-Correlation-Id` value to follow one request across both APIs:

```bash
podman compose logs -f authapi quotesapi   # or: docker compose logs -f authapi quotesapi
```

## Traces

Server spans wrap every handler (health probes excluded), via the otelhttp handler. When
a collector is attached, a browser request that signs in and then reads a quote produces
two spans — one per API — sharing the trace context; there is never a quotesapi→authapi
span, because token validation is local.

## Metrics

Custom meter: `code.examples.go.quotes` (`telemetry.MeterName`). One counter per use
case, incremented with an `outcome` tag:

| Counter | Outcome vocabulary |
|---------|--------------------|
| `auth.login.count` | `success` \| `failure` |
| `auth.validate.count` | `success` \| `failure` |
| `quotes.random.count` | `success` \| `not_found` \| `error` |
| `quotes.getbyid.count` | `success` \| `not_found` \| `error` |
| `quotes.list.count` | `success` \| `invalid` \| `error` |
| `quotes.create.count` | `success` \| `invalid` \| `conflict` \| `error` |

The quotes outcome values are the ErrorOr vocabulary ported verbatim from the .NET
meter. The counters are incremented at the transport — the v3 service records every
call's outcome (it is the single place every v3 request passes through), the auth
transport records login/validate around its service calls.

## Correlation

Every request carries `X-Correlation-Id` — the client's value when supplied, a minted
32-hex-character id otherwise — echoed on the response, stored in the request context,
stamped on the active span (`correlation.id`) and W3C baggage, attached to the request
log line, and carried into every problem body's `correlationId` extension. The login
response returns it, and the SPA reuses it on quote calls — the cross-API trail a human
or a collector can follow.
