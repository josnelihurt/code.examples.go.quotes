#!/usr/bin/env bash
# Thin exec over the code.examples.ci submodule
# (ci/conventions/scripts/check-conventions.sh) for manual invocations
# (--branch / --title / --range); the local hooks exec their own canonical
# scripts from the same submodule, and CI calls the SHA-pinned action
# directly. No network and no gh needed once the submodule is initialized.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
canonical="${ROOT}/ci/conventions/scripts/check-conventions.sh"
if [[ ! -f "${canonical}" ]]; then
  echo "the code.examples.ci submodule is not initialized — run: git submodule update --init ci" >&2
  exit 2
fi
exec bash "${canonical}" "$@"
