# ADR 0001 — Orchestration: compose spec + Traefik v3 edge (replaces Aspire + YARP)

* Status: accepted · Date: 2026-08-25*

## Context

The .NET original (`code.examples.net.quotes`, `src/AppHost/AppHost.cs` on `origin/main`)
orchestrates via Aspire: postgres (+pgweb), auth-api, quotes-api, Vite/pnpm frontend, Docsify docs on :3001, YARP routing `/api/v1/auth/*` + `/api/v0..v3/quotes/*`; publish mode is
`AddDockerComposeEnvironment`. The Go port (user-confirmed) uses compose + Traefik on podman
5.7.1/podman-compose (macOS) and docker compose (ubuntu CI). .NET pins: 18.3 / 0.17.0 / 2.3-preview.

## Decision

1. **Compose + Traefik v3.7 edge.** v3.7 (2026-05-05) is the only actively supported v3 line; pin
   patch `v3.7.11`. Entrypoint `web` on in-network :80 (host 8080); no TLS locally.
2. **File provider, not docker labels.** Labels are the compose community default but scatter routing and require the engine socket (path differs: docker CI vs rootless podman). Two static routes
   read better as one watched dynamic file — route-table parity with the YARP block — no socket,
   identical on both engines (`--providers.docker.enabled=false`).
3. **Aspire mapping**: `WaitFor(db)` → `depends_on: {condition: service_healthy}` + `pg_isready`
   healthcheck; connection strings → `DATABASE_URL` env (pgx URL form); `AddDatabase` → `POSTGRES_DB=
   quotesdb` (no createdb step); secret `jwt-signing-key` → shared `${JWT_SIGNING_KEY:?}` env;
   lifecycle → profiles (core always, `dev` = pgweb+docs, `fullstack` = frontend); publish mode →
   this file *is* the artifact. Postgres stays **volume-less**: migrate+seed per run, deterministic e2e.
4. **Route parity** (`PathPrefix` = prefix+subtree, path unchanged like YARP): `/api/v1/auth` → authapi, `/api/v3/quotes` → quotesapi; no middleware, no TLS. (YARP also exposes v0–v2 quotes.)
5. **Docs service**: `nginx:1.31.4-alpine3.24` — ~4-line conf (root + Docsify `try_files $uri /index.html`), host port **3001** parity. Rejected: `caddy:2.11.4-alpine` (equal, less ubiquitous);
   docsify-cli/node (heavier; mirrors Aspire run mode only).
6. **Frontend dev**: `node:24.19.0-alpine3.23` — Node 24 is active LTS (Aug 2026), satisfies engines `^20.19 || >=22.12`, matches CI. pnpm 11 (current 11.24.0) via `corepack enable` honoring
   `packageManager` (ships in Node 24, dropped in 25+). Vite proxy targets `http://edge:80`.
7. **Podman caveats**: prefer `podman compose` (shim delegates to docker-compose when installed — CI-parity semantics); podman-compose `depends_on: condition` has bug history (#866, #1119, #1422 —
   `podman wait --condition=healthy` once succeeded for created-not-started), so Go apps still retry
   DB connects. Named volumes fine on both; host port 80 fine (podman VM binds privileged on macOS).
   Healthchecks use in-image tools (`pg_isready`, busybox `wget` — keep alpine Go runtimes).

## Alternatives

- **Caddy edge**: simpler config, but Traefik's watched file provider is closer to YARP parity.
- **nginx edge**: fine statically; manual reloads on topology change. **No proxy**: loses the one-origin `/api/*` contract. **Kubernetes**: too much machinery for 6 containers.

## Consequences

- One reviewable topology file + one route file; no Aspire SDK/dashboard; dev topology equals the publish artifact; route edits are watched file edits — fine at two routes.
- docker/podman compatibility surface = healthchecks + `depends_on` conditions; CI (docker compose) gates.

## .NET mapping

| Aspire / .NET (origin/main) | Go repo (compose + Traefik) |
|---|---|
| `DistributedApplication.CreateBuilder` + `AddDockerComposeEnvironment` | compose file = dev topology = publish artifact |
| `AddPostgres("postgres")` + `WithPgWeb()` + `AddDatabase("quotesdb")` | `postgres` svc (`pg_isready` hc, `POSTGRES_DB: quotesdb`) + `pgweb` (`dev` profile) |
| `WithReference(quotesDb)` (conn-string) | `DATABASE_URL: postgres://…@postgres:5432/quotesdb` |
| `WaitFor(quotesDb)` | `depends_on: {postgres: {condition: service_healthy}}` |
| `AddParameter("jwt-signing-key", secret)` | `${JWT_SIGNING_KEY:?}` env shared by both APIs |
| `WithHttpHealthCheck("/health")` | service `healthcheck:` hitting `/health` |
| `AddYarp` routes `/api/v1/auth`, `/api/v3/quotes` | Traefik file-provider routers (same prefixes) |
| `AddViteApp(...).WithPnpm()` | `web` service (`fullstack` profile), Vite proxy → `http://edge:80` |
| Docsify `docsify-cli serve -p 3001` | nginx on host port **3001** (`dev` profile) |

## Pins

Verified 2026-08-25 via Docker Hub + GitHub releases; no `latest`-style tags:

- `docker.io/traefik/traefik:v3.7.11` — v3.7 only actively supported v3 line (3.6 security ends 2026-08-16)
- `docker.io/library/postgres:18.6-alpine3.23` — current 18.x (.NET pins 18.3; same PG 18 line)
- `docker.io/sosedoff/pgweb:0.17.0` — exact parity with .NET pin; current upstream release
- `docker.io/library/nginx:1.31.4-alpine3.24` — docs static server
- `docker.io/library/node:24.19.0-alpine3.23` — frontend dev; pnpm 11 via Corepack. YARP image dropped

## Appendix: skeletal `docker-compose.yaml` + `traefik/dynamic.yml`

```yaml
x-api-env: &api-env { JWT_SIGNING_KEY: "${JWT_SIGNING_KEY:?set in .env}" }
services:
  edge:
    image: docker.io/traefik/traefik:v3.7.11
    command: >-
      --entryPoints.web.address=:80 --providers.docker.enabled=false --providers.file.directory=/etc/traefik --providers.file.watch=true
    ports: ["8080:80"] # gateway parity: http://localhost:8080
    volumes: ["./traefik:/etc/traefik:ro"]
  postgres: # no volume: migrate+seed per run (e2e determinism)
    image: docker.io/library/postgres:18.6-alpine3.23
    environment: { POSTGRES_DB: quotesdb, POSTGRES_PASSWORD: quotes }
    healthcheck: { test: ["CMD-SHELL", "pg_isready -U postgres -d quotesdb"], interval: 2s, timeout: 3s, retries: 30 }
  authapi:
    build: ./auth-api # Go, alpine base (busybox wget for healthcheck)
    environment: <<: *api-env
    depends_on: { postgres: { condition: service_healthy } }
    healthcheck: { test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"], interval: 5s, retries: 12 }
  quotesapi:
    build: ./quotes-api
    environment: { <<: *api-env, DATABASE_URL: "postgres://postgres:quotes@postgres:5432/quotesdb" }
    depends_on: { postgres: { condition: service_healthy } }
  docs: # Docsify parity on :3001
    image: docker.io/library/nginx:1.31.4-alpine3.24
    volumes: ["./docs:/usr/share/nginx/html:ro"]
    ports: ["3001:80"]
    profiles: [dev]
  pgweb:
    image: docker.io/sosedoff/pgweb:0.17.0
    command: "--url=postgres://postgres:quotes@postgres:5432/quotesdb?sslmode=disable"
    ports: ["8081:8081"]
    depends_on: { postgres: { condition: service_healthy } }
    profiles: [dev]
  web: # fullstack profile
    image: docker.io/library/node:24.19.0-alpine3.23
    volumes: ["./frontend:/app"]
    command: sh -c "cd /app && corepack enable && pnpm install && pnpm dev --host 0.0.0.0"
    environment: { VITE_PROXY_TARGET: "http://edge:80" }
    ports: ["3000:3000"]
    depends_on: [authapi, quotesapi]
    profiles: [fullstack]
```

```yaml
# traefik/dynamic.yml (whole route table, watched live)
http:
  routers:
    auth:   { rule: "PathPrefix(`/api/v1/auth`)",   service: authapi }
    quotes: { rule: "PathPrefix(`/api/v3/quotes`)", service: quotesapi }
  services:
    authapi:   { loadBalancer: { servers: [{ url: "http://authapi:8080" }] } }
    quotesapi: { loadBalancer: { servers: [{ url: "http://quotesapi:8080" }] } }
```
