# auth context

The auth bounded context — the deliberately thin sibling of
[internal/quotes](../quotes/README.md): no database, no catalog invariants, the same
layering (domain / application / infrastructure / api) enforced by the same depguard
rules.

| Layer | Folder | Owns |
|-------|--------|------|
| Domain | [domain](domain/) | the scope vocabulary (`quotes:read` / `quotes:write`), `IssuedToken`, `ValidateResult`, the credential/token ports, `InvalidCredentials` |
| Application | [application](application/) | `AuthService` — the one multi-method application service (login + validate) over the two ports |
| Infrastructure | [infrastructure](infrastructure/) | `NewHardcodedCredentialStore` (the two documented local users; refuses Production), `NewJwtTokenService` (golang-jwt HS256 mint + validate), `NewRateLimiter` (per-key fixed window) |
| API | [api](api/) | the login/validate endpoints: camelCase DTOs, RFC 9457 problems, the rate-limit rejection |

Scope differentiation is real: the credential store returns granted scopes, the token
service mints exactly those claims — `jrb` holds read+write, `reader` read-only, so a
403 is reachable by any client of the seed. Token validation normalizes the scope claim
whether it travels as one space-separated string (RFC 8693) or a JSON array.

The users, the development signing key and the Production startup refusals are
documented in exactly one place: [docs/dev-credentials.md](../../docs/dev-credentials.md).
The decision record: [ADR 0006](../../docs/adr/0006-auth-and-errors.md).
