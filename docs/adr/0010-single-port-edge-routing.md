# ADR 0010 — Single-port edge routing for dev surfaces

* Status: accepted · Date: 2026-08-26 · Amends: [0001](0001-orchestration-compose-traefik.md)*

## Context

ADR 0001 published the dev stack on separate host ports for .NET Aspire parity: Traefik
on `:8080` for APIs, docs on `:3001`, pgweb on `:8081`, and the Vite SPA on `:5173`.
That split worked but meant four front doors on a laptop. The SPA already proxied API
calls to the edge in compose; the browser still saw multiple origins when opening docs
or pgweb alongside the app. The Traefik dashboard was left disabled in favour of
container logs.

## Decision

1. **Traefik remains the only published host port** (`QUOTES_EDGE_PORT`, default `8080`).
2. **The default route is the Traefik dashboard**: `api.dashboard: true` (insecure off);
   `/` redirects to `/dashboard/`; `api@internal` serves `/dashboard` and Traefik's
   control `/api` at priority 40 — below auth/quotes (100) so `/api/v1` and `/api/v3`
   stay with the Go services.
3. **Path-prefix routing** in `traefik/dynamic.yml` joins optional dev surfaces behind
   the edge:
   - `/docs` → `docs:80` (StripPrefix `/docs`)
   - `/pgweb` → `pgweb:8081` (StripPrefix `/pgweb`)
   - `/app` → `frontend:5173` (fullstack; Vite `base=/app/`, no strip)
4. **API routes unchanged** at priority 100: `/api/v1/auth`, `/api/v3/quotes` — backends
   still see the full request path.
5. **Remove host `ports:` publishes** from `docs`, `pgweb`, and `frontend` in
   `docker-compose.yaml`.
6. **Docsify `relativePath: true`** in `docs/index.html` so client-side navigation works
   under `/docs/`.
7. **Shared SPA stays host-configurable**: compose sets `VITE_DEV_ORIGIN` and
   `VITE_BASE_PATH=/app/`. The SPA (`frontend/vite.config.ts`, `main.tsx` basename)
   also accepts `VITE_SERVER_HOST` and `VITE_HMR_*`. All unset keeps Aspire /
   `pnpm run dev` on stock `:5173` at `/`.

## Consequences

- One URL moves the whole stack: `QUOTES_EDGE_PORT=8082 ./scripts/start.sh` retargets
  dashboard, APIs, docs, pgweb, and SPA together.
- Opening `http://localhost:8080/` always lands on the Traefik dashboard, even on
  `--core` (no frontend required).
- Profile-only backends that are not running return 502 on their paths; acceptable for
  `--core`.
- Departs from .NET Aspire's separate docs/pgweb/Vite ports; the compose file remains
  the publish artifact, now with a single host binding.
- `./scripts/e2e.sh` and BDD are unchanged: they boot loopback API processes and/or hit
  the edge on `:8080` directly, not the optional dev-surface paths.

## Path map

| Path | Backend | Profile |
|------|---------|---------|
| `/` → `/dashboard/` | Traefik dashboard | core |
| `/dashboard`, Traefik `/api/*` | `api@internal` | core |
| `/api/v1/auth` | authapi | core |
| `/api/v3/quotes` | quotesapi | core |
| `/docs` | docs (nginx) | dev |
| `/pgweb` | pgweb | dev |
| `/app` | frontend (Vite) | fullstack |
