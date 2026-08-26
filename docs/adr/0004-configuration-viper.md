# ADR 0004 — Configuration: Viper with typed structs and fail-fast validation

* Status: accepted · Date: 2026-08-25*

## Context

Viper was selected by owner decision; this ADR fixes the usage details. The .NET kit (`src/ServiceDefaults/JwtAuthExtensions.cs`, `src/Auth/Auth.Infrastructure/JwtTokenService.cs`,
`src/Auth/Auth.Api/RateLimitingExtensions.cs`) reads configuration through `IConfiguration` with layered
providers: JSON files, environment variables using `__` as the section separator (`Jwt:SigningKey` becomes
`Jwt__SigningKey`), and user-secrets in Development. Misconfiguration fails at boot — a missing signing key,
an HMAC key under 32 bytes, or the documented dev key in Production each throw before the host serves.
The Go port needs the same layering, the same env-var names (compose/Kubernetes manifests already use
`Jwt__SigningKey`, `ConnectionStrings__quotesdb`, `RateLimiting__Auth__PermitLimit`), and the same fail-fast stance.

## Decision

Use `github.com/spf13/viper` v1.21.0, constructed once in the composition root — never the package-level global.

1. **Layering (lowest to highest):** `v.SetDefault` for every platform key -> optional config file
   (`AddConfigPath`/`SetConfigFile`; an absent file is not an error) -> `v.AutomaticEnv()`. This mirrors
   defaults -> appsettings.json -> environment.
2. **Env names:** `v.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))` maps the config path
   `jwt.signingkey` to `JWT__SIGNINGKEY` — byte-for-byte parity with .NET's `__` separator (Viper
   uppercases lookups, so Linux env vars match). **Accept both forms:** also register
   `v.BindEnv("jwt.signingkey", "JWT__SIGNINGKEY", "JWT_SIGNINGKEY")` — `BindEnv` checks each name in
   order and the first present wins. Canonical form is `JWT__SIGNINGKEY`; the flat `JWT_SIGNINGKEY`
   alias is declared only for the handful of platform keys, next to the config struct.
3. **Typed structs:** one `v.Unmarshal(&cfg)` into a `Config` struct (`mapstructure` tags) with
   `Jwt` (SigningKey, Issuer, Audience, ExpiresInSeconds), `ConnectionStrings` (QuotesDb) and
   `RateLimiting.Auth` (PermitLimit, WindowSeconds). Known Viper caveat: `Unmarshal` walks only
   `AllKeys()` (defaults + file keys) and silently misses env-only keys — registering a default for
   every platform key (step 1) is what makes env-only overrides reach the struct. Do not skip defaults.
4. **Fail-fast validation at boot:** `cfg.Validate()` runs in `main` before serving and returns fatal
   errors for: missing or <32-byte `Jwt.SigningKey`; the dev signing key when running as production;
   non-positive `RateLimiting.Auth.PermitLimit`; missing `ConnectionStrings.QuotesDb` in services that
   own a database. Parity with the `InvalidOperationException` boot failures in `JwtAuthExtensions`.
5. **No global singleton:** the `*viper.Viper` instance is discarded after `Unmarshal` + `Validate`;
   the immutable `*Config` is injected into constructors. No `viper.Get*` outside the composition root —
   the hidden global is exactly what the .NET kit avoids by injecting `IOptions<T>`.

## Alternatives

- **koanf v2.3.6** — cleaner layering (explicit providers), no default global, small dependency footprint.
  Rejected: the owner decided Viper, and its docs/ecosystem familiarity outweigh koanf's tidier API here.
- **stdlib only** (`os.Getenv` + `encoding/json`) — roughly 100 lines and zero dependencies, but it
  reimplements env-name mapping, the defaults layer and nested decoding, and each consumer would hand-roll
  it differently. Kept as the fallback if the dependency ever becomes a liability.

## Consequences

- The env-var contract is identical to the .NET kit; the same compose/Kubernetes manifests configure both ports.
- Every new platform key must be added in three places — defaults, struct field, validation — the same
  checklist the .NET kit imposes (defaults, options class, boot guard).
- `mapstructure` tags (not `json`) bind fields; a missing tag silently drops a key, which validation
  catches at boot rather than at first use.
- Viper's post-v1.20 dependency slimming keeps the transitive set modest; versions pin via `go.mod`.

## .NET mapping

| .NET (reference repo)                          | Go (this ADR)                                          |
|------------------------------------------------|--------------------------------------------------------|
| `Configuration.GetSection("Jwt")["SigningKey"]` | `cfg.Jwt.SigningKey` (typed struct)                    |
| appsettings.json / appsettings.{Env}.json       | optional file layer via `AddConfigPath`                |
| env `Jwt:SigningKey` = `Jwt__SigningKey`        | replacer `.`->`__` + `BindEnv` alias `JWT_SIGNINGKEY`  |
| `IOptions<AuthRateLimitOptions>`                | injected immutable `*Config`                           |
| boot `InvalidOperationException` guards         | `cfg.Validate()` fatal at startup                      |
| `Configuration["OTEL_EXPORTER_OTLP_ENDPOINT"]`  | `os.Getenv` (OTel SDK reads its own env natively)      |
| user-secrets (Development)                      | git-ignored local env vars/file; never the file layer  |

## Pins

- `github.com/spf13/viper` **v1.21.0** (2025-09-08; latest stable per proxy.golang.org)
- evaluated, not adopted: `github.com/knadh/koanf/v2` v2.3.6
