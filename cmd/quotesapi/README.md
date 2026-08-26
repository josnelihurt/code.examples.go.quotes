# quotesapi

The quotes service's **composition root** (`main.go`) and the place the two bounded
contexts meet: it adapts the auth context's token validator onto the v3 transport's
`Authenticator` port (`bearerAuthenticator`), because the layering guard forbids either
context from importing the other. The domain and use cases live in
[internal/quotes](../../internal/quotes/README.md); the platform kit in
[internal/platform](../../internal/platform/README.md).

## What it serves

The v3 proto transport (grpc-go server on a loopback listener, grpc-gateway runtime in
front — see [ADR 0002](../../docs/adr/0002-v3-transport-grpc-gateway.md)):

- `GET /api/v3/quotes/random`, `GET /api/v3/quotes/{id}`, `GET /api/v3/quotes` — bearer token with `quotes:read`
- `POST /api/v3/quotes` — bearer token with `quotes:write`
- `GET /openapi/v3.json` (the embedded frozen document), `GET /scalar` (reference UI)
- `GET /health` — readiness **including a `SELECT 1` round-trip against the catalog**; `GET /alive` — liveness

## Boot sequence

`config.Load` → slog logger → `telemetry.Setup` → open the catalog: `Migrate` (embedded,
advisory-locked, idempotent) with bounded retry — compose ordering is a convenience, the
boot itself tolerates a database still becoming healthy → the pgx pool → the repository,
the token validator, the four use cases → the grpc server + gateway + host mux → serve
with graceful shutdown (HTTP drain first, then `GracefulStop`).

`cmd/quotesapi/wire_test.go` boots this composition in-process and pins the wire
semantics the .NET drift tests pinned: the error envelope with `"details":[]`, the
gRPC→HTTP status table, paging defaults, the `X-Correlation-Id` echo, the 401
problem+json with `WWW-Authenticate`, and create answering 200 with no `Location`.
