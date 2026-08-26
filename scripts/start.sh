#!/usr/bin/env bash
# Brings the compose topology (ADR 0001) up and proves the edge round-trip.
# Engine-agnostic: docker compose when available (CI parity), podman compose
# otherwise. Profile selection:
#
#   ./scripts/start.sh                # dev: postgres + both APIs + edge + docs + pgweb
#   ./scripts/start.sh --core         # minimal: postgres + both APIs + edge
#   ./scripts/start.sh --fullstack    # dev + the Vite frontend
#   ./scripts/start.sh down           # tear it all down (containers + networks)
#
# The post-up verification mints a real token through the edge and reads a
# page of quotes with it — edge routing, both APIs, migration-at-boot seeding
# and JWT parity, one round-trip. Credentials come from the environment
# (QUOTES_DEV_USERNAME/QUOTES_DEV_PASSWORD); the documented development
# values live in docs/dev-credentials.md.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

compose() { # docker compose when present, the podman shim otherwise
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    docker compose "$@"
  else
    podman compose "$@"
  fi
}

is_docker_compose() {
  command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1
}

compose_up() { # `up --wait` semantics on both engines
  if is_docker_compose; then
    compose "${args[@]}" up -d --wait # docker compose blocks on healthchecks natively
    return
  fi
  # podman-compose (1.5) has no --wait flag: start detached, then block until
  # every container the project started is running and, where a healthcheck is
  # configured, healthy. This only keeps the probes below from racing the
  # healthchecks — a stack that comes up broken still fails the probes loudly.
  compose "${args[@]}" up -d
  local deadline=$((SECONDS + 600)) id state health pending
  while :; do
    pending=0
    while IFS= read -r id; do
      [ -n "${id}" ] || continue
      # {{if .State.Health}} guards the template: podman inspect errors on a
      # nil pointer for containers without a healthcheck (docs, pgweb), and
      # that error must read as "no healthcheck", not as "not healthy".
      state="$(podman inspect --format '{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}' "${id}" 2>/dev/null || echo "missing|")"
      health="${state#*|}"
      state="${state%%|*}"
      case "${state}:${health}" in
        running:healthy | running:) ;; # healthy, or no healthcheck configured
        *) pending=$((pending + 1)) ;;
      esac
    done < <(compose ps -q)
    [ "${pending}" -eq 0 ] && return 0
    if [ "${SECONDS}" -ge "${deadline}" ]; then
      echo "timed out waiting for ${pending} container(s) to become healthy" >&2
      return 1
    fi
    printf '  waiting for %d container(s) to pass their healthchecks (%ss elapsed)\n' "${pending}" "${SECONDS}"
    sleep 3
  done
}

profiles=(dev)
mode=up
for arg in "$@"; do
  case "${arg}" in
    --core) profiles=(core) ;;
    --fullstack) profiles=(dev fullstack) ;;
    down) mode=down ;;
    *) echo "usage: $0 [--core|--fullstack] [down]" >&2; exit 2 ;;
  esac
done

# The core services carry no profile stanza, so they run under every
# selection; the flags only control which optional profiles join.
args=()
for profile in "${profiles[@]}"; do
  if [ "${profile}" != core ]; then
    args+=(--profile "${profile}")
  fi
done

if [ "${mode}" = down ]; then
  # --profile flags must cover every profile that may be running, or compose
  # leaves its services up; dev and fullstack are the whole optional set.
  compose --profile dev --profile fullstack down --remove-orphans --volumes
  exit 0
fi

compose_up

echo
echo "verifying the edge round-trip (http://localhost:8080)..."
if [ -z "${QUOTES_DEV_USERNAME:-}" ] || [ -z "${QUOTES_DEV_PASSWORD:-}" ]; then
  echo "  skipped: set QUOTES_DEV_USERNAME/QUOTES_DEV_PASSWORD (see docs/dev-credentials.md) to run the authenticated probes"
  echo "  unauthenticated probes:"
  curl -fsS -o /dev/null -w '  edge     GET  /            -> %{http_code}\n' http://localhost:8080/ || true
  curl -fsS -o /dev/null -w '  edge     GET  /api/v3/quotes (no token) -> %{http_code}\n' "http://localhost:8080/api/v3/quotes?page=1&page_size=2" || true
  exit 0
fi

login() {
  curl -fsS -X POST http://localhost:8080/api/v1/auth/login \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${QUOTES_DEV_USERNAME}\",\"password\":\"${QUOTES_DEV_PASSWORD}\"}"
}

# A successful login answers an accessToken; a minted JWT is three
# dot-separated base64 segments, which is enough shape to trust here.
token="$(login | sed -E 's/.*"accessToken" *: *"([^"]+)".*/\1/')"
case "${token}" in
  *.*.*) echo "  login    POST /api/v1/auth/login -> 200 (token minted)" ;;
  *) echo "  login through the edge failed: no accessToken in the response" >&2; exit 1 ;;
esac

body="$(curl -fsS -H "Authorization: Bearer ${token}" "http://localhost:8080/api/v3/quotes?page=1&page_size=2")"
items="$(printf '%s' "${body}" | sed -E 's/.*"items" *: *\[//; s/\].*//' | grep -o '"id"' | wc -l | tr -d ' ')"
echo "  quotes   GET  /api/v3/quotes?page=1&page_size=2 -> 200 (${items} items: routing, migration seeding and JWT parity all hold)"
