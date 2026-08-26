#!/usr/bin/env python3
"""Check that everything the documentation cites actually exists in the code.

Backticked tokens in the pages the documentation pass authors are matched
against the repository: repo paths must resolve, route strings must appear in
source, and type / method / constant / metric names must be defined somewhere.
Fenced blocks are ignored (they are examples, not citations).

A token that is deliberately absent — a tool to install, a value the design
chose not to commit — belongs in ALLOW below, with the reason.

Usage: verify-docs-refs.py <repo-root>
"""
import re
import subprocess
import sys
import pathlib

CODE = re.compile(r'`([^`\n]+)`')
DIRISH = re.compile(
    r'^(cmd|internal|tests|docs|frontend|scripts|contracts|migrations|traefik|\.github)/[\w./\-*{}]+$'
)
ROOT_FILE = re.compile(r'^(\w+file[\w.]*|Makefile|go\.(mod|sum)|AGENTS\.md|\.golangci\.yml)$', re.I)
ROUTE = re.compile(
    r'^/(?:api/v[0-9]+/[\w/{}\-]+|openapi/[\w./\-]+|scalar|health|alive)$'
)
IDENT = re.compile(
    r'^(?:[A-Z][A-Za-z0-9]{2,}(?:\.[A-Za-z][A-Za-z0-9]*)*'
    r'|[a-z][a-z0-9]*(?:\.[A-Za-z][A-Za-z0-9]*)+'
    r'|(?:[a-z][a-z0-9_]*\.)+[a-z][a-z0-9_]*'
    r'|[A-Z][A-Z0-9_]{2,}'
    r'|quotes:(?:read|write))$'
)
FENCE = re.compile(r'^```')

SOURCE_SUFFIXES = {
    '.go', '.proto', '.sql', '.yaml', '.yml', '.sh', '.env', '.json',
    '.ts', '.tsx', '.html', '.feature', '.mod', '.conf', '.cfg',
}
SKIP_DIRS = ('node_modules', 'dist', 'coverage', '.git', 'bin', 'playwright-report', 'test-results')

# Authored pages: the component readmes next to the source and the docs pages
# this pass writes with code citations in them.
PAGE_GLOBS = (
    "cmd/**/README.md", "internal/**/README.md", "tests/README.md",
    "contracts/README.md",
)
EXTRA_PAGES = (
    "docs/architecture.md", "docs/data-storage.md", "docs/testing.md",
    "docs/local-dev.md", "docs/observability.md", "docs/api.md",
    "docs/system-design.md",
)

# Deliberately absent from the code; each entry needs a reason. Gitignored paths
# do not belong here — is_generated() lets a page name build output.
ALLOW = {
    'pnpm',            # the package manager executable, not a repository artifact
    'ServiceDefaults', # the .NET repo's platform-kit project, cited as narrative
    # inside the frontend submodule: present in any recursive checkout, absent
    # from the CI docs job's checkout (submodules stay unfetched there)
    'frontend/package.json',
}


def doc_set(root):
    """The pages the documentation pass authors."""
    found = []
    for pattern in PAGE_GLOBS:
        found += sorted(root.glob(pattern))
    for extra in EXTRA_PAGES:
        page = root / extra
        if page.exists():
            found.append(page)
    return found


def source_blob(root):
    chunks = []
    for path in root.rglob('*'):
        if not path.is_file() or path.suffix not in SOURCE_SUFFIXES:
            continue
        if any(part in SKIP_DIRS for part in path.relative_to(root).parts):
            continue
        try:
            chunks.append(path.read_text(encoding='utf-8', errors='replace'))
        except OSError:
            pass
    return '\n'.join(chunks)


def prose_of(path):
    kept, in_fence = [], False
    for line in path.read_text(encoding='utf-8', errors='replace').splitlines():
        if FENCE.match(line):
            in_fence = not in_fence
        elif not in_fence:
            kept.append(line)
    return '\n'.join(kept)


def exists(root, doc, candidate):
    """A path may be repo-root relative or relative to the page citing it."""
    return (root / candidate).exists() or (doc.parent / candidate).exists()


def is_generated(root, candidate):
    """Gitignored paths are build output a page may legitimately name."""
    return any(
        subprocess.run(
            ['git', '-C', str(root), 'check-ignore', '-q', '--no-index', spelling],
            capture_output=True,
        ).returncode == 0
        for spelling in (candidate, candidate.rstrip('/') + '/')
    )


def main():
    root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else '.').resolve()
    docs = doc_set(root)
    if not docs:
        print("  no component documentation found")
        return 0

    blob = source_blob(root)
    problems = []
    counts = {'paths': 0, 'routes': 0, 'identifiers': 0}

    for doc in docs:
        rel = doc.relative_to(root)
        for token in (t.strip() for t in CODE.findall(prose_of(doc))):
            if DIRISH.match(token) or ROOT_FILE.match(token):
                counts['paths'] += 1
                probe = token.rstrip('/')
                if any(ch in probe for ch in '*{'):
                    probe = re.split(r'[*{]', probe)[0].rstrip('/')
                if (probe and token not in ALLOW
                        and not exists(root, doc, probe)
                        and not is_generated(root, probe)):
                    problems.append(f"{rel}: path does not exist -> `{token}`")
            elif ROUTE.match(token):
                counts['routes'] += 1
                segments = token.strip('/').split('/')
                probe = '/'.join(segments[1:3]) if len(segments) > 2 else segments[-1]
                if token not in blob and probe not in blob:
                    problems.append(f"{rel}: route not found in source -> `{token}`")
            elif IDENT.match(token) and not token.endswith('.'):
                base = token.split('<')[0].split('.')[-1]
                if len(base) < 4:
                    continue
                counts['identifiers'] += 1
                if base not in blob and token not in ALLOW:
                    problems.append(f"{rel}: identifier not found in source -> `{token}`")

    print(f"  {counts['paths']} paths, {counts['routes']} routes, "
          f"{counts['identifiers']} identifiers across {len(docs)} pages")
    for problem in problems:
        print(f"  FAIL {problem}")
    return 1 if problems else 0


if __name__ == '__main__':
    sys.exit(main())
