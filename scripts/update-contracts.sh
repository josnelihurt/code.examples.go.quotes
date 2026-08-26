#!/usr/bin/env bash
# Freeze the v3 quotes OpenAPI document into docs/openapi/ via Dockerfile.build
# (hermetic: pinned buf + protoc-gen-openapiv2 over the committed proto sources).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# podman or docker, whichever is installed (DOCKER overrides). CI's contract-drift
# job uses plain `docker build --output`, but a podman machine on macOS runs remote
# mode where --output is unsupported — the create + cp extraction below works on
# both, which is also how the .NET repo's script extracts the scratch stage.
if [ -n "${DOCKER:-}" ]; then
  :
elif command -v podman >/dev/null 2>&1; then
  DOCKER=podman
elif command -v docker >/dev/null 2>&1; then
  DOCKER=docker
else
  echo "neither podman nor docker found — set DOCKER" >&2
  exit 1
fi

# The tag is namespaced per worktree (8-hex hash of the repo root) so two checkouts
# building their contracts export at once don't race the same image tag.
# CONTRACTS_IMAGE_TAG overrides.
SUFFIX="$(printf '%s' "${ROOT}" | shasum | cut -c1-8)"
IMAGE_TAG="${CONTRACTS_IMAGE_TAG:-localhost/go-quotes-contracts:export-${SUFFIX}}"
OUT_DIR="${ROOT}/docs/openapi"

cd "${ROOT}"
mkdir -p "${OUT_DIR}"

echo "==> Building contracts export image (${DOCKER})"
"${DOCKER}" build -f Dockerfile.build --target contracts -t "${IMAGE_TAG}" .

cid="$("${DOCKER}" create "${IMAGE_TAG}")"
cleanup() {
  "${DOCKER}" rm -f "${cid}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> Copying frozen document to docs/openapi"
"${DOCKER}" cp "${cid}:/docs/openapi/quotes-v3.openapi.json" "${OUT_DIR}/quotes-v3.openapi.json"

echo "Updated:"
echo "  ${OUT_DIR}/quotes-v3.openapi.json"
echo "Done. Review the diff before committing — the proto is the input, the"
echo "document is build output."
