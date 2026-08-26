# Development credentials

The single source of truth for every credential the stack uses **in non-Production environments**. These values are scaffolding so the compose topology boots offline; every one of them is refused or rejected the moment the environment is Production. Nothing on this page may be reused for anything real.

The CI `secrets-hygiene` job enforces this consolidation: the literals below may appear here, in the one code location that implements them, and in tests that must authenticate — nowhere else.

## Local users (scaffolding identity)

Implemented by `internal/auth/infrastructure/credentialstore.go` (`NewHardcodedCredentialStore`), which refuses to run when the environment is Production.

| User | Password | Scopes |
|------|----------|--------|
| `jrb` | `supersecret` | `quotes:read`, `quotes:write` |
| `reader` | `readsecret` | `quotes:read` |

## JWT development signing key

The compose topology (ADR 0001) shares one HS256 key between `authapi` (mints) and `quotesapi` (validates) through the `AUTH_SIGNING_KEY` environment variable, which defaults to a public local-development value — deliberately **not** the `config.DevelopmentSigningKey` constant, so a compose boot never mistakes itself for a `dotnet run`-style dev boot. Override it per-machine:

```bash
AUTH_SIGNING_KEY=any-value-of-at-least-32-bytes ./scripts/start.sh
```

Any value of at least 32 bytes is accepted outside Production (`internal/platform/config`); Production startup rejects the documented development key and every shorter value.

## E2E throwaway values

The full-stack Playwright run (`scripts/e2e.sh`) needs its own `E2E_SIGNING_KEY` — any
value of at least 32 characters, never reused from this page:

```bash
export E2E_SIGNING_KEY="local-e2e-<your-random-32-plus-chars>"
./scripts/e2e.sh
```

CI synthesizes an ephemeral key from the run id. The throwaway catalog's connection
values (loopback port, user, password, database) live in `scripts/e2e.env` — the one
copy shared by the script and the CI job; they are disposable by design.

## Rules

1. New non-Production credentials are documented **here** or they do not exist; the grep gate fails CI on literals outside this file, the implementing code, and tests.
2. Production must never boot on anything from this page — the startup guards are load-bearing, not decorative.
