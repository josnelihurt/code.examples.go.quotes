# API

The v3 surface is defined entirely by the proto contract —
[`contracts/quotes/v3/quotes_v3.proto`](../contracts/quotes/v3/quotes_v3.proto) — whose
`google.api.http` rules drive the routing through grpc-gateway. This page is the guide;
the frozen, generated OpenAPI document is the machine-readable truth
([api contracts](../contracts/api-contracts.md)).

## Reference surfaces

- `GET /openapi/v3.json` on quotesapi — the frozen document served verbatim (the same
  embedded bytes the `contract-drift` CI job diffs)
- `GET /scalar` on quotesapi — the interactive Scalar reference pointing at that document
- With the dev profile up: the Docsify site at `http://localhost:8080/docs/` carries the same
  [Scalar page](scalar/) (the explicit `index.html` matters under nginx)

Those first two are mounted **on the service**, and the edge routes only the two `/api`
prefixes — so under `./scripts/start.sh` the Docsify page is the way in. The specs reach
the service routes directly instead: `tests/bdd/compose.bdd.yaml` publishes the API
containers on host ports for exactly that, which is also how the `/health` and `/alive`
probes are exercised.

Scalar is the interactive client for humans; the specs (`./scripts/bdd.sh`) are the
automated one. Neither is required by the other.

## Authentication

Every quotes route requires a bearer JWT minted by the auth API. Scopes are real: reads
need `quotes:read`, create needs `quotes:write`; a valid token without the route's scope
is an empty-body 403. The challenges, answered before the gateway so their shapes are
byte-identical to the .NET pipeline:

- no token → `401` problem+json (`auth.token_missing`) with `WWW-Authenticate: Bearer`
- rejected token → `401` problem+json (`auth.token_invalid`) with
  `WWW-Authenticate: Bearer error="invalid_token"`

Getting a token: `POST /api/v1/auth/login` with the documented development users
([dev credentials](dev-credentials.md)) — the response carries `accessToken`,
`expiresIn`, `username` and the `correlationId`. Both auth endpoints are rate-limited
per client IP (fixed window; over-limit answers `429` problem+json with
`auth.rate_limited`).

## The quotes routes

| Route | Scope | Notes |
|-------|-------|-------|
| `GET /api/v3/quotes/random` | `quotes:read` | 200 with the quote; empty catalog answers the not-found envelope |
| `GET /api/v3/quotes/{id}` | `quotes:read` | unknown id → `404` envelope, gRPC code 5 |
| `GET /api/v3/quotes?page=1&page_size=20` | `quotes:read` | 1-based; defaults 1/20, bounds: page ≤ 10000, page size ≤ 100; violations → `400` envelope, code 3; response carries `items`, `page`, `pageSize`, `totalItems`, `totalPages` in stable catalog order |
| `POST /api/v3/quotes` | `quotes:write` | body `{"text": …, "author": …}`; answers **200 with the created quote — no `Location`** (the annotation rules cannot express 201 + Location; the deliberate drift this transport exists to show); invalid text/author → `400` code 3; near-duplicate → `409` code 6 |

Every response echoes `X-Correlation-Id` (the client's value or a minted one).

## Error envelopes — two shapes on purpose

**The v3 quotes surface uses the gRPC status envelope** (the gateway's stock error
handler): gRPC code as `code`, the domain error's description as `message`, and
`details` always present (an empty array — the marshaler knob pinned in ADR 0002). A
real unknown-id answer, exactly as the wire tests pin it:

```json
{
  "code": 5,
  "message": "Quote not found.",
  "details": []
}
```

The gRPC→HTTP status mapping is the stock table (5→404, 6→409, 3→400, 16→401, …), and
the machine-readable domain code deliberately does not travel — the canonical carrier
would be an ErrorInfo detail, which the .NET transcoding writer cannot render either
(ADR 0002).

**The auth API and the pre-gateway 401/403 use RFC 9457 problem+json**
(`application/problem+json`): `type`/`title`/`status` plus the `errorCode`,
`correlationId` and `traceId` extensions, validation failures under `errors`. A login
with a wrong password:

```json
{
  "type": "https://tools.ietf.org/html/rfc9110#section-15.5.2",
  "title": "Unauthorized",
  "status": 401,
  "detail": "Invalid credentials.",
  "errorCode": "auth.invalid_credentials",
  "correlationId": "…",
  "traceId": "…"
}
```

## When the contract changes

The proto is the input, the frozen document is build output:

```bash
./scripts/update-contracts.sh   # regenerate docs/openapi/quotes-v3.openapi.json hermetically
```

Review the diff and commit it with the proto change; CI's `contract-drift` job fails on
any mismatch. The full pipeline record: [ADR 0003](adr/0003-openapi-generation-pipeline.md).
