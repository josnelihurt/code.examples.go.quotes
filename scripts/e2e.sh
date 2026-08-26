#!/usr/bin/env bash
# Runs the frontend submodule's full-stack Playwright suite against THIS
# backend: builds both Go binaries, starts a throwaway PostgreSQL for the
# quotes catalog (the API migrates + seeds it at boot), starts authapi and
# quotesapi as local processes on loopback ports, then hands over to
# Playwright — which boots only the Vite dev server (tests/e2e/
# playwright.config.ts points its proxy targets at the two API processes).
#
# The API transport finding this wiring lives around: the SPA selects its
# API version in src/api/client.ts — sessionStorage key "apiVersion",
# default 'v1', no env hook — and this backend serves the v3 transport only,
# so the playwright config excludes the scenarios that pin v0/v1/v2 by name
# and file. The v3 journey (random quote, catalog, publish), the sign-in
# journeys and sign-out run for real end to end.
#
# E2E_SIGNING_KEY (>= 32 chars) is REQUIRED: it is the HS256 key both API
# processes share. CI synthesizes an ephemeral one from the run id; locally
# export any value of at least 32 characters (see docs/dev-credentials.md).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=images.env
source "${ROOT}/scripts/images.env"
# shellcheck source=e2e.env
source "${ROOT}/scripts/e2e.env"

PG_NAME="quotes-e2e-pg"
AUTH_PORT="${E2E_AUTH_PORT:-5801}"
QUOTES_PORT="${E2E_QUOTES_PORT:-5802}"
VITE_PORT="${E2E_VITE_PORT:-5803}"

if [ -z "${E2E_SIGNING_KEY:-}" ] || [ "${#E2E_SIGNING_KEY}" -lt 32 ]; then
  echo "E2E_SIGNING_KEY must be set to a value of at least 32 characters before running e2e" >&2
  echo "(see docs/dev-credentials.md; CI generates an ephemeral one), for example:" >&2
  echo '  export E2E_SIGNING_KEY="local-e2e-<your-random-32-plus-chars>"' >&2
  exit 1
fi

# Honors E2E_CONTAINER_RUNTIME (podman by default on macOS laptops); both
# CLIs accept these args.
RUNTIME="${E2E_CONTAINER_RUNTIME:-podman}"
if ! command -v "${RUNTIME}" >/dev/null 2>&1 && command -v docker >/dev/null 2>&1; then
  RUNTIME=docker
fi

wait_for() { # wait_for <url> <budget-seconds> — 2xx within the budget or fail
  local url="$1" budget="$2" i
  for i in $(seq 1 "${budget}"); do
    if curl -fsS -o /dev/null "${url}" 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for ${url}" >&2
  return 1
}

echo "building the APIs..."
mkdir -p "${ROOT}/bin"
go build -o "${ROOT}/bin/authapi" ./cmd/authapi
go build -o "${ROOT}/bin/quotesapi" ./cmd/quotesapi

echo "starting the throwaway catalog (image pinned in scripts/images.env)..."
"${RUNTIME}" pull "${POSTGRES_IMAGE}"
# Only ever removes this checkout's own leftover from a crashed previous run.
"${RUNTIME}" rm -f "${PG_NAME}" >/dev/null 2>&1 || true
"${RUNTIME}" run -d --name "${PG_NAME}" \
  -e POSTGRES_USER="${E2E_PG_USER}" \
  -e POSTGRES_PASSWORD="${E2E_PG_PASSWORD}" \
  -e POSTGRES_DB="${E2E_PG_DATABASE}" \
  -p "127.0.0.1:${E2E_PG_PORT}:5432" \
  "${POSTGRES_IMAGE}"
for _ in $(seq 1 60); do
  if "${RUNTIME}" exec "${PG_NAME}" pg_isready -U "${E2E_PG_USER}" -d "${E2E_PG_DATABASE}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

LOG_DIR="$(mktemp -d)"
PIDS=()
cleanup() {
  for pid in "${PIDS[@]:-}"; do
    kill "${pid}" >/dev/null 2>&1 || true
  done
  "${RUNTIME}" rm -f "${PG_NAME}" >/dev/null 2>&1 || true
  # The binaries are reproducible build outputs; the logs are diagnostics —
  # keep the path on stdout so a failed run can be inspected.
  echo "e2e API logs kept at ${LOG_DIR}"
}
trap cleanup EXIT

echo "starting authapi on 127.0.0.1:${AUTH_PORT} and quotesapi on 127.0.0.1:${QUOTES_PORT}..."
SERVER__ADDRESS="127.0.0.1:${AUTH_PORT}" \
  JWT__SIGNINGKEY="${E2E_SIGNING_KEY}" \
  "${ROOT}/bin/authapi" >"${LOG_DIR}/authapi.log" 2>&1 &
PIDS+=($!)
SERVER__ADDRESS="127.0.0.1:${QUOTES_PORT}" \
  JWT__SIGNINGKEY="${E2E_SIGNING_KEY}" \
  CONNECTIONSTRINGS__QUOTESDB="postgres://${E2E_PG_USER}:${E2E_PG_PASSWORD}@127.0.0.1:${E2E_PG_PORT}/${E2E_PG_DATABASE}?sslmode=disable" \
  "${ROOT}/bin/quotesapi" >"${LOG_DIR}/quotesapi.log" 2>&1 &
PIDS+=($!)

wait_for "http://127.0.0.1:${AUTH_PORT}/alive" 30
# Includes the at-boot migration + seed of the throwaway database.
wait_for "http://127.0.0.1:${QUOTES_PORT}/health" 120

# Hand the per-run values to the playwright config (tests/e2e/
# playwright.config.ts reads these with the defaults above as fallbacks).
export E2E_AUTH_PORT="${AUTH_PORT}"
export E2E_QUOTES_PORT="${QUOTES_PORT}"
export E2E_VITE_PORT="${VITE_PORT}"
# CI=true keeps the run deterministic everywhere: pnpm stays non-interactive,
# playwright writes the html report and boots its own Vite dev server.
# NODE_PATH lets the config file (outside the frontend package, loaded by
# `playwright test` as CommonJS) resolve playwright-bdd from the frontend's
# node_modules — bddgen's own loader already resolves there.
export CI=true
export NODE_PATH="${ROOT}/frontend/node_modules"

cd "${ROOT}/frontend"
pnpm install --frozen-lockfile
if [ "$(uname)" = Linux ]; then
  pnpm exec playwright install --with-deps chromium
else
  # --with-deps is the Linux package install; macOS has no equivalent step.
  pnpm exec playwright install chromium
fi
pnpm exec bddgen -c ../tests/e2e/playwright.config.ts
pnpm exec playwright test -c ../tests/e2e/playwright.config.ts
