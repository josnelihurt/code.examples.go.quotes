#!/usr/bin/env bash
# Opts this clone into the repository's local git hooks (.githooks): the
# commit-msg and pre-push validators, whose canonical scripts run from the
# code.examples.ci submodule on disk. Pure git configuration — no
# package-manager lifecycle involved, matching the frontend's deliberate
# hookless posture (see code.examples.frontend.quotes's docs).
#
#   ./scripts/setup-git-hooks.sh                        # enable
#   git config --unset core.hooksPath                    # undo
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "${ROOT}"

git submodule update --init ci
git config core.hooksPath .githooks
echo "Local hooks enabled (.githooks: commit-msg, pre-push — canonical scripts from the ci submodule)."
echo "Undo with: git config --unset core.hooksPath"
