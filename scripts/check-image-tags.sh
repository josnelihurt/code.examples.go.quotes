#!/usr/bin/env bash
# Fails when any container image reference in the compose topology drifts from
# the pins in scripts/images.env. There is no hosting SDK whose packages could
# serve as the authority (the .NET original cross-checked Aspire.Hosting.*),
# so scripts/images.env IS the source: docker-compose.yaml's `image:` lines and
# the two API Dockerfiles' `FROM` lines must quote exactly the pinned tags — a
# hand-edit that bypasses the pin file, or a pin bump that misses a referencing
# file, turns this gate red with the expected/actual pair. Floating references
# (`:latest`, or no tag at all) are rejected outright, as are pins no file
# references anymore.
#
#   ./scripts/check-image-tags.sh   # exit 0 = in sync, exit 1 = drift, exit 2 = cannot check
#
# Pure shell on purpose: the CI job needs no toolchain beyond checkout, and the
# script stays bash-3.2-clean (macOS's /bin/bash) so laptops and CI run the
# same code. LC_ALL=C keeps BSD grep and GNU grep byte-identical.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"
export LC_ALL=C

fail=0

KEYS="POSTGRES_IMAGE PGWEB_IMAGE TRAEFIK_IMAGE NGINX_IMAGE NODE_IMAGE GOLANG_BUILDER_IMAGE ALPINE_RUNTIME_IMAGE"

# --- repo side: the shared pin file -------------------------------------------------

pin() { # pin KEY -> the value from scripts/images.env (exit 2 if absent)
  local value
  value="$(grep -E "^${1}=" scripts/images.env | head -1 | cut -d= -f2- || true)"
  if [[ -z "${value}" ]]; then
    echo "error: scripts/images.env has no ${1} entry" >&2
    exit 2
  fi
  printf '%s' "${value}"
}

pins=""
for key in ${KEYS}; do
  pins+="${key}|$(pin "${key}")"$'\n'
done

# --- topology side: what docker-compose.yaml and the Dockerfiles actually run -------

unquote() { # strip one layer of matching single or double quotes, if present
  sed -E "s/^'(.*)'\$/\1/; s/^\"(.*)\"\$/\1/"
}

refs="$(
  {
    grep -hE '^[[:space:]]*image:[[:space:]]' docker-compose.yaml \
      | sed -E 's/^[[:space:]]*image:[[:space:]]*//' | unquote
    grep -hE '^FROM[[:space:]]' Dockerfile.authapi Dockerfile.quotesapi \
      | sed -E 's/^FROM[[:space:]]+//; s/[[:space:]]+AS[[:space:]].*$//' | unquote
  } | sort -u
)"

if [[ -z "${refs}" ]]; then
  echo "error: no image references found in docker-compose.yaml or the Dockerfiles" >&2
  exit 2
fi

# --- verdict ------------------------------------------------------------------------

echo "Container image pins — scripts/images.env vs the compose topology:"
used=" "
while IFS= read -r ref; do
  matched=""
  while IFS='|' read -r key value; do
    [[ -n "${key}" ]] || continue
    if [[ "${ref}" == "${value}" ]]; then
      matched="${key}"
      break
    fi
  done <<<"${pins}"

  if [[ -n "${matched}" ]]; then
    used+="${matched} "
    printf '  %-22s ok      %s\n' "${matched}" "${ref}"
  elif [[ "${ref}" == *:latest ]] || [[ "${ref}" != *:* ]]; then
    printf '  %-22s FLOAT   %s (floating tag — pin it in scripts/images.env)\n' unpinned "${ref}"
    fail=1
  else
    printf '  %-22s DRIFT   %s is not pinned in scripts/images.env\n' unpinned "${ref}"
    fail=1
  fi
done <<<"${refs}"

for key in ${KEYS}; do
  case "${used}" in
    *" ${key} "*) ;;
    *) printf '  %-22s UNUSED  %s (no image:/FROM references this pin)\n' "${key}" "$(pin "${key}")"; fail=1 ;;
  esac
done

if [[ "${fail}" -ne 0 ]]; then
  echo
  echo "Drift: update scripts/images.env and the referencing file(s) together (the"
  echo "compose topology must quote the pinned tags literally), or re-pin the entry."
  exit 1
fi
