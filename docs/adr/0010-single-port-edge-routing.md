# ADR 0010 — Single-port edge routing for dev surfaces

* Status: accepted · Date: 2026-08-26 · Amends: [0001](0001-orchestration-compose-traefik.md)*

## Context

ADR 0001 published the dev stack on separate host ports for .NET Aspire parity: Traefik
on `:8080` for APIs, docs on `:3001`, pgweb on `:8081`, and the Vite SPA on `:5173`.
That split worked but meant four front doors on a laptop. The SPA already proxied API
calls to the edge in compose; the browser still saw multiple origins when opening docs
or pgweb alongside the app.

## Decision

1. **Traefik remains the only published host port** (`QUOTES_EDGE_PORT`, default `8080`).
2. **Path-prefix routing** in `traefik/dynamic.yml` joins optional dev surfaces behind
   the edge:
   - `/docs` → `docs:80` (StripPrefix `/docs`)
   - `/pgweb` → `pgweb:8081` (StripPrefix `/pgweb`)
   - `/` → `frontend:5173` (fullstack profile; catch-all at low router priority)
3. **API routes unchanged** at priority 100: `/api/v1/auth`, `/api/v3/quotes` — backends
   still see the full request path.
4. **Remove host `ports:` publishes** from `docs`, `pgweb`, and `frontend` in
   `docker-compose.yaml`.
5. **Docsify `relativePath: true`** in `docs/index.html` so client-side navigation works
   under `/docs/`.
6. **Vite HMR through the edge**: `VITE_DEV_ORIGIN` (compose) sets `server.origin` and
   `hmr.clientPort` in `frontend/vite.config.ts` so hot reload works when the browser
   talks to `:8080`.

## Consequences

- One URL moves the whole stack: `QUOTES_EDGE_PORT=8082 ./scripts/start.sh` retargets
  APIs, docs, pgweb, and SPA together.
- Dev-only stacks without the fullstack profile: open `/docs/` directly; `/` returns 502
  until the frontend profile is up (no static root redirect — it would break fullstack).
- Profile-only backends that are not running return 502 on their paths; acceptable for
  `--core`.
- Departs from .NET Aspire's separate docs/pgweb/Vite ports; the compose file remains
  the publish artifact, now with a single host binding.
- `./scripts/e2e.sh` and BDD are unchanged: they boot loopback API processes and/or hit
  the edge on `:8080` directly, not the optional dev-surface paths.

## Path map

| Path | Backend | Profile |
|------|---------|---------|
| `/api/v1/auth` | authapi | core |
| `/api/v3/quotes` | quotesapi | core |
| `/docs` | docs (nginx) | dev |
| `/pgweb` | pgweb | dev |
| `/` | frontend (Vite) | fullstack |
