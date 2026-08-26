#!/usr/bin/env bash
# Runs the godog specification suite (tests/bdd) against the compose stack:
# postgres + both APIs + the Traefik edge, brought up under the quotes-bdd
# project name with the tests/bdd/compose.bdd.yaml overlay (host ports for
# the services' documentation/health surfaces, a spec-sized login rate
# limit). The suite itself skips in `go test ./...` when nothing answers —
# this script is the path that guarantees something does.
#
#   ./scripts/bdd.sh                # bring the stack up, run the suite, tear it down
#   ./scripts/bdd.sh --no-teardown  # keep the stack up after the run (dev loop)
#   ./scripts/bdd.sh down           # tear the specification stack down
#
# Ports: the edge keeps docker-compose.yaml's QUOTES_EDGE_PORT default (8080)
# unless BDD_EDGE_PORT is set; the overlay's service ports follow
# BDD_QUOTES_API_PORT/BDD_AUTH_API_PORT (8090/8091). The BDD_* values are
# exported so the suite's own defaults line up with whatever this script
# chose. AUTH_SIGNING_KEY flows into the stack and into BDD_SIGNING_KEY so
# scenarios that mint tokens directly sign with the key quotesapi validates.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

PROJECT="${BDD_COMPOSE_PROJECT:-quotes-bdd}"
TEARDOWN=1
MODE=run
for arg in "$@"; do
  case "${arg}" in
    --no-teardown) TEARDOWN=0 ;;
    down) MODE=down ;;
    *) echo "usage: $0 [--no-teardown] [down]" >&2; exit 2 ;;
  esac
done

compose() { # docker compose when present, the podman shim otherwise
  if is_docker_compose; then
    docker compose "$@"
  else
    podman compose "$@"
  fi
}

is_docker_compose() {
  command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1
}

FILES=(-f docker-compose.yaml -f tests/bdd/compose.bdd.yaml)

down() {
  compose "${FILES[@]}" -p "${PROJECT}" down --remove-orphans --volumes
}

if [ "${MODE}" = down ]; then
  down
  exit 0
fi

# The stack's signing key and the suite's minting key are one value: compose
# defaults it to the public local-development key, and the suite defaults
# BDD_SIGNING_KEY to the same, so the export is only load-bearing when the
# operator overrode AUTH_SIGNING_KEY.
export BDD_SIGNING_KEY="${BDD_SIGNING_KEY:-${AUTH_SIGNING_KEY:-public-local-compose-signing-key-0000000000000000}}"
export BDD_BASE_URL="${BDD_BASE_URL:-http://localhost:${BDD_EDGE_PORT:-8080}}"
export QUOTES_EDGE_PORT="${BDD_EDGE_PORT:-8080}"
export BDD_COMPOSE_PROJECT="${PROJECT}"

if [ "${TEARDOWN}" -eq 1 ]; then
  trap down EXIT
fi

# `up --wait` semantics on both engines: docker compose blocks on the
# healthchecks natively; podman-compose (1.5) has no --wait flag, so start
# detached and poll until every container is running and healthy (the same
# compromise scripts/start.sh makes — a stack that comes up broken still
# fails the suite loudly right after). The project is torn down first so
# every run sees the deterministic from-scratch catalog (the property the
# e2e suite depends on, kept here too): podman-compose's `up` on an existing
# project reuses its containers, database state included.
up() {
  compose "${FILES[@]}" -p "${PROJECT}" down --remove-orphans --volumes >/dev/null 2>&1 || true
  if is_docker_compose; then
    compose "${FILES[@]}" -p "${PROJECT}" up -d --build --wait postgres authapi quotesapi edge
    return
  fi
  compose "${FILES[@]}" -p "${PROJECT}" up -d --build postgres authapi quotesapi edge
  local deadline=$((SECONDS + 600)) id state health pending
  while :; do
    pending=0
    while IFS= read -r id; do
      [ -n "${id}" ] || continue
      state="$(podman inspect --format '{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}' "${id}" 2>/dev/null || echo "missing|")"
      health="${state#*|}"
      state="${state%%|*}"
      case "${state}:${health}" in
        running:healthy | running:) ;;
        *) pending=$((pending + 1)) ;;
      esac
    done < <(compose "${FILES[@]}" -p "${PROJECT}" ps -q)
    [ "${pending}" -eq 0 ] && return 0
    if [ "${SECONDS}" -ge "${deadline}" ]; then
      echo "timed out waiting for ${pending} container(s) to become healthy" >&2
      return 1
    fi
    printf '  waiting for %d container(s) to pass their healthchecks (%ss elapsed)\n' "${pending}" "${SECONDS}"
    sleep 3
  done
}

up

# The suite's health scenario drives the same containers through
# BDD_COMPOSE_ENGINE; export the engine this script detected.
if is_docker_compose; then
  export BDD_COMPOSE_ENGINE=docker
else
  export BDD_COMPOSE_ENGINE=podman
fi

go test ./tests/bdd/... -count=1 -v
