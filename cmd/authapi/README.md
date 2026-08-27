# authapi

The auth service's **composition root** — the only place config, telemetry, the auth
adapters and the transport meet. Layout: [`main.go`](main.go) (OS signals + boot
orchestration), [`wire.go`](wire.go) (`newHandler`, shared with the wire tests).
Everything below the composition root lives in
[internal/auth](../../internal/auth/README.md); the shared host kit in
[internal/platform](../../internal/platform/README.md).

## What it serves

- `POST /api/v1/auth/login` — `{username, password}` → `{accessToken, correlationId, expiresIn, username}`; 401 problem+json on bad credentials, 429 when the per-IP fixed window is exhausted
- `POST /api/v1/auth/validate` — RFC 7662-style introspection: valid and invalid tokens both answer 200 `{valid, username}`, only a missing token is a 400
- `GET /health`, `GET /alive` — readiness / liveness (unauthenticated, untraced)

Both auth routes sit behind the rate limiter; the endpoints' exact wire shapes (headers,
error envelopes) are owned by [internal/auth/api](../../internal/auth/api/).

## Boot sequence

`config.Load` (fail-fast: missing/short signing key, dev key in Production) → slog JSON
logger → `telemetry.Setup` (no-op without `OTEL_EXPORTER_OTLP_ENDPOINT`) → `newHandler`
(hardcoded credential store → JWT → AuthService → rate limiter → mux, wrapped
outside-in with server spans, correlation and request logging).

Tokens are HS256 with the configured issuer/audience (`config.Jwt`); the same key
validates them in quotesapi. The users and the signing-key rules live in
[docs/dev-credentials.md](../../docs/dev-credentials.md).

## Tests

`cmd/authapi/main_test.go` (`package main`) pins the composition's HTTP surface via
`newHandler` — the login/validate round-trips through the real handler stack. Run with
`make test`.
