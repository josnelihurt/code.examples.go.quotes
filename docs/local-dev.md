# Local development

## Prerequisites

- **Go 1.27** (the `go` directive in `go.mod`; the API images' builder stage pins the
  same version in `scripts/images.env`)
- **Podman** (or Docker) — `./scripts/start.sh`, `./scripts/bdd.sh` and
  `./scripts/update-contracts.sh` are engine-agnostic; `make test-db` points
  `DOCKER_HOST` at the podman machine socket when one is running
- **pnpm** — only for the full-stack e2e run and rendering the docs' mermaid diagrams;
  the exact version is pinned by `packageManager` in `frontend/package.json` (inside the
  submodule)
- A checkout with the frontend submodule present:
  `git clone --recurse-submodules …` (or `git submodule update --init`)

## Start the stack

```bash
./scripts/start.sh               # dev: postgres + both APIs + edge + docs + pgweb
./scripts/start.sh --core        # postgres + both APIs + edge
./scripts/start.sh --fullstack   # dev + the Vite dev server (:5173)
./scripts/start.sh down          # tear it all down
```

`start.sh` verifies the edge round-trip after the stack is healthy: it logs in through
`http://localhost:8080` with `QUOTES_DEV_USERNAME` / `QUOTES_DEV_PASSWORD` (the
documented development users — [dev credentials](dev-credentials.md)) and reads one
page of quotes with the minted token. Without those variables it prints the
unauthenticated probes instead.

| Service | URL |
|---------|-----|
| edge | `http://localhost:8080` (`QUOTES_EDGE_PORT` moves it) |
| docs | `http://localhost:3001` |
| pgweb | `http://localhost:8081` (pre-connected to the catalog) |
| SPA | `http://localhost:5173` (fullstack profile; pick `v3` in the UI's version switcher) |

Both APIs bind `:8080` **in-container** (`SERVER__ADDRESS`); the edge is the only
published front door. The one shared secret is `AUTH_SIGNING_KEY` — the compose default
is the public local-development value; override it with any value of at least 32 bytes
(see [dev credentials](dev-credentials.md)).

## Common tasks

| Task | Command |
|------|---------|
| Build | `make build` |
| Units + wire tests | `make test` |
| Database integration tests | `make test-db` |
| Specs against the compose stack | `make bdd` (or `./scripts/bdd.sh --no-teardown` to keep the stack up) |
| Full-stack e2e | `./scripts/e2e.sh` (needs `E2E_SIGNING_KEY`) |
| Lint + layering guard | `./scripts/lint.sh` (`--fix` where it can) |
| Regenerate the frozen OpenAPI document | `./scripts/update-contracts.sh` |
| Regenerate the v3 Go contract code | `make contracts-go` (needs buf) |
| Verify this documentation set | `./scripts/verify-docs.sh` |
| Sync `go.mod`/`go.sum` | `make tidy` |

## Git hooks (optional)

```bash
./scripts/setup-git-hooks.sh        # enable (commit-msg + pre-push)
git config --unset core.hooksPath   # undo
```

The hooks validate the commit subject and the branch name locally; the canonical rules
live in [code.examples.ci](https://github.com/josnelihurt/code.examples.ci) and CI
enforces them regardless — [contributing](contributing.md) has the full reference.

## Troubleshooting

- **Port 8080 busy** — `QUOTES_EDGE_PORT=8082 ./scripts/start.sh`.
- **The stack came up but the probes failed** — the boot itself retries the database
  for a bounded budget; check the quotesapi container's log stream for the migration
  output, and remember `./scripts/start.sh down` removes volumes too, so the next boot
  is a fresh seeded catalog.
- **Two checkouts on one machine** — the specs run under their own compose project
  (`quotes-bdd`) and `update-contracts.sh` tags its export image per worktree, so
  concurrent runs do not race; `start.sh`'s ports are fixed, so serialize dev stacks.
