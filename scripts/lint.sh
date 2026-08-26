#!/usr/bin/env bash
# Lints the Go module with the committed .golangci.yml — the same gate the CI
# lint job runs, including the depguard layering rules (ADR 0009).
#
#   ./scripts/lint.sh          # check only, non-zero exit on violations
#   ./scripts/lint.sh --fix    # let golangci-lint apply the fixes it can
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${ROOT}"

exec golangci-lint run "$@"
