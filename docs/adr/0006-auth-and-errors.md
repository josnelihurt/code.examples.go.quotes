# ADR 0006 — Auth, problem+json errors, rate limiting, routing, health

* Status: accepted · Date: 2026-08-25*

## Context

The .NET kit validates JwtBearer HS256 with issuer/audience and per-scope policies (`JwtAuthExtensions.cs`;
1-minute `ClockSkew`; boot guards: key >= 32 bytes, dev key refused in Production). 401s are RFC 9457 problem+json
(`auth.token_missing`/`auth.token_invalid`) with `WWW-Authenticate`. Every failure — middleware (401/429) and ErrorOr
results alike — shares one envelope (`ProblemDetailsBuilder`): status/title/detail/type plus `errorCode`/`correlationId`
extensions and RFC 9110 type links. A fixed-window per-IP limiter (`RateLimiting:Auth`, default 10/30s,
queue 0) rejects with 429 `auth.rate_limited`. `/health` readiness runs `SELECT 1` under a 5-second wall clock; `/alive` is the liveness pair.

## Decision

1. **JWT: `github.com/golang-jwt/jwt/v5` v5.3.1.** Sign HS256 via
   `jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)` where claims embed
   `jwt.RegisteredClaims` (`iss`/`aud`/`exp`/`sub`) plus `Name` and `Scope` string fields (`name`,
   `scope`). Validate with `jwt.ParseWithClaims` + `jwt.WithIssuer`, `jwt.WithAudience`,
   `jwt.WithLeeway(time.Minute)` (ClockSkew parity), `jwt.WithValidMethods([]string{"HS256"})`
   (blocks alg confusion), `jwt.WithExpirationRequired()`. Scope authorization stays per-API:
   policies are declared at composition as checks on `claims.Scope`, mirroring
   `AddStandardJwtAuthentication(("quotes:read","quotes:read"), ...)`. Boot guards verbatim:
   `len(key) >= 32` bytes, and refuse dev key `AspireQuotesPoc-Dev-Signing-Key-32chars!` in production.
2. **RFC 9457: hand-rolled helper (~40 lines).** One `problem` package:
   `Write(w, status, errorCode, detail, req)` sets `Content-Type: application/problem+json` and emits
   `type/title/detail/status` plus extensions `errorCode`, `correlationId`, and `traceId` (the span
   context's TraceID, omitted when the trace is invalid — how ASP.NET Core stamps `traceId`). Type
   links reuse the .NET table: 401 -> rfc9110#section-15.5.2, 403 -> 15.5.4, 404 -> 15.5.5, 409 ->
   15.5.10, 429 -> 15.5.14, 500 -> 15.6.1, default 15.5.1. Validation failures add an `errors` map
   keyed by field. **Why hand-rolled:** ASP.NET Core ships ProblemDetails; Go has no de-facto
   equivalent — the few pkg.go.dev candidates are single-file, low-maintenance helpers that would
   still need our extension members and type-link table, and framework-tied generators assume their
   router. The envelope is the port's public contract; owning 40 lines beats a dependency that
   shapes it.
3. **Rate limiting: hand-rolled fixed window per IP.** `map[ip]struct{start time.Time; count int}`
   under one mutex; a request in the current window increments, an expired window resets; over
   `PermitLimit` (from `RateLimiting:Auth`, [ADR 0004](0004-configuration-viper.md)) -> 429 problem `auth.rate_limited`, no queueing
   (QueueLimit 0 parity). **`golang.org/x/time/rate` rejected:** token bucket with burst-and-refill
   semantics and no window boundary — the observable contract differs (an 11th call inside the window
   can pass right after a refill tick, and window-reset tests flake). The .NET `FixedWindowRateLimiter`
   is the contract; the primitive is ~30 lines with lazy reset bounding the map.
4. **Router: stdlib `net/http.ServeMux`** — Go 1.22+ method+wildcard patterns on Go 1.27:
   `mux.HandleFunc("POST /login", ...)`, `GET /quotes/{id}` + `r.PathValue("id")`; a wrong method
   against a registered path answers 405 automatically. chi v5.3.2 buys subrouters and middleware
   groups the kit does not need; parity lives at the endpoint contract, not the framework.
5. **Correlation middleware:** read `X-Correlation-Id`; if absent or blank, mint 16 crypto-random
   bytes hex-encoded (32 chars — the `Guid.ToString("N")` shape); echo on the response; store in
   context (`FromContext` mirrors `GetCorrelationId`); stamp span attribute + baggage and the logger.
6. **Health endpoints:** `/health` (readiness) executes `SELECT 1` under a hard 5-second wall clock —
   run the query in a goroutine and `select` against a timer, abandoning frozen sockets (the .NET
   `Task.WhenAny` guard: cooperative cancellation cannot revoke a blocked read); failure or timeout
   answers 503 JSON. `/alive` (liveness) is dependency-free. Both register before auth/tracing
   middleware and stay unauthenticated, in every environment.

## Alternatives

- JWT: `lestrrat-go/jwx/v3` — far broader JWK/JWE support than an HS256 kit needs.
- Rate limiting: `x/time/rate` (wrong semantics, above); `go-redis/redis_rate` (distributed, no Redis here).
- Router: chi v5.3.2, gorilla/mux — capability beyond the kit's needs. Errors: framework problem-details
  middleware (router-coupled) or tiny unmaintained helpers (above).

## Consequences

- One error envelope across auth, rate limiting and handlers — clients parse a single shape, as in the reference.
- The auth middleware owns the 401 body (`WWW-Authenticate: Bearer` or `Bearer error="invalid_token"` +
  problem), keeping token failures indistinguishable from the .NET kit's.
- The limiter is in-process (multiple replicas need a shared store — accepted, matching .NET); ServeMux
  patterns require Go >= 1.22 (pinned toolchain 1.27).

## .NET mapping

| .NET (reference repo)                            | Go (this ADR)                                      |
|--------------------------------------------------|----------------------------------------------------|
| `TokenValidationParameters` + ClockSkew 1 min     | `ParseWithClaims` + `WithLeeway(time.Minute)`      |
| `JwtSecurityTokenHandler.CreateToken` (HS256)     | `NewWithClaims(HS256).SignedString`                |
| `RequireClaim("scope", s)` policies               | per-scope checks on `Claims.Scope` at composition  |
| `OnChallenge` 401 problem + WWW-Authenticate      | auth middleware writes problem+json + header       |
| `ProblemDetailsBuilder`/`ProblemDetailsFactory`   | hand-rolled `problem` package                      |
| `FixedWindowRateLimiter` per-IP partition         | mutex + map fixed window, `PermitLimit` from config|
| `MapHealthChecks` /health + /alive                | net/http handlers; readiness `SELECT 1`, 5s budget |
| `UseCorrelationId` / `GetCorrelationId`           | correlation middleware + `FromContext`             |

## Pins

- `github.com/golang-jwt/jwt/v5` **v5.3.1** (2026-01-28; latest stable per proxy.golang.org)
- stdlib only: `net/http`, `log/slog`, `sync`, `crypto/rand`
- evaluated, not adopted: `github.com/go-chi/chi/v5` v5.3.2; `golang.org/x/time/rate` (semantics mismatch)
