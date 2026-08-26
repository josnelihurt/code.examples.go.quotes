#!/usr/bin/env bash
# Verification gate for the documentation set (docs/, the README story and the
# component readmes) — every documented claim is mechanically verified:
#
#   1. links     every markdown link and heading anchor resolves
#   2. refs      every backticked repo path, route and identifier exists in the code
#   3. mermaid   every mermaid fence renders (needs network + pnpm; SKIP_MERMAID=1 to skip)
#
# Usage: ./scripts/verify-docs.sh [--skip-mermaid]
set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

SKIP_MERMAID="${SKIP_MERMAID:-0}"
if [[ "${1:-}" == "--skip-mermaid" ]]; then
  SKIP_MERMAID=1
elif [[ -n "${1:-}" ]]; then
  echo "Usage: $0 [--skip-mermaid]" >&2
  exit 2
fi

status=0

echo "==> links and anchors"
python3 scripts/verify-docs-links.py "${ROOT}" || status=1

echo "==> code references"
python3 scripts/verify-docs-refs.py "${ROOT}" || status=1

echo "==> mermaid diagrams"
if [[ "${SKIP_MERMAID}" == "1" ]]; then
  echo "  skipped (SKIP_MERMAID=1)"
else
  # mmdc renders every fence in a markdown file in one browser session; a syntax error
  # in any diagram fails the file. Output is thrown away — only the exit code matters.
  work="$(mktemp -d)"
  trap 'rm -rf "${work}"' EXIT
  total=0
  while IFS= read -r page; do
    count="$(grep -c '^```mermaid' "${page}" || true)"
    [[ "${count}" -eq 0 ]] && continue
    total=$((total + count))
    if pnpm dlx @mermaid-js/mermaid-cli@11 -i "${page}" -o "${work}/$(echo "${page}" | tr '/.' '__').md" \
        >"${work}/mmdc.log" 2>&1; then
      printf '  %2s OK      %s\n' "${count}" "${page}"
    else
      printf '  %2s FAIL    %s\n' "${count}" "${page}"
      sed -n '1,20p' "${work}/mmdc.log"
      status=1
    fi
    # --others --exclude-standard so a brand-new page — the whole point of a documentation
    # pass — is checked before it is ever staged.
  done < <(git ls-files --cached --others --exclude-standard \
             'docs/*.md' 'cmd/*/README.md' 'internal/**/README.md' 'tests/README.md' 'README.md' | sort -u)
  echo "  ${total} diagrams checked"
fi

echo
if [[ "${status}" -eq 0 ]]; then
  echo "documentation gate: PASS"
else
  echo "documentation gate: FAIL" >&2
fi
exit "${status}"
