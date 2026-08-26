#!/usr/bin/env python3
"""Check every markdown link and heading anchor in the documentation set.

Covers the root README, every page under docs/ (ADRs included), the
per-component readmes next to the source, the contracts pointer page, and the
frontend submodule's README. Fenced code blocks are ignored so illustrative
links are not checked.

Usage: verify-docs-links.py <repo-root>
"""
import re
import sys
import pathlib
import unicodedata

LINK = re.compile(r'(?<!!)\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)')
FENCE = re.compile(r'^```')
HEADING = re.compile(r'^(#{1,6})\s+(.*?)\s*#*\s*$')


def doc_set(root):
    """The pages the documentation pass owns or links from."""
    found = [root / "README.md", *sorted(root.glob("docs/**/*.md"))]
    found += sorted(root.glob("cmd/**/README.md"))
    found += sorted(root.glob("internal/**/README.md"))
    found += sorted(root.glob("tests/README.md"))
    found += sorted(root.glob("contracts/*.md"))
    frontend = root / "frontend" / "README.md"
    if frontend.exists():
        found.append(frontend)
    return [p for p in found if p.exists()]


def slugify(text):
    """Approximate GitHub's heading-anchor slug."""
    text = re.sub(r'`([^`]*)`', r'\1', text)
    text = re.sub(r'\[([^\]]*)\]\([^)]*\)', r'\1', text)
    kept = [
        ch for ch in text.strip().lower()
        if ch.isalnum() or ch in ('-', '_', ' ') or unicodedata.category(ch).startswith('M')
    ]
    return re.sub(r'-+', '-', ''.join(kept).replace(' ', '-'))


def headings_of(path, _cache={}):
    """Anchor slugs a page exposes, with duplicates suffixed the way GitHub does."""
    if path in _cache:
        return _cache[path]
    slugs, seen, in_fence = set(), {}, False
    for line in path.read_text(encoding='utf-8', errors='replace').splitlines():
        if FENCE.match(line):
            in_fence = not in_fence
        elif not in_fence:
            match = HEADING.match(line)
            if match:
                slug = slugify(match.group(2))
                nth = seen.get(slug, 0)
                seen[slug] = nth + 1
                slugs.add(slug if nth == 0 else f"{slug}-{nth}")
    _cache[path] = slugs
    return slugs


def prose_of(path):
    """File contents with fenced blocks blanked out."""
    lines, in_fence, kept = path.read_text(encoding='utf-8', errors='replace').splitlines(), False, []
    for line in lines:
        if FENCE.match(line):
            in_fence = not in_fence
            kept.append('')
        else:
            kept.append('' if in_fence else line)
    return '\n'.join(kept)


def main():
    root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else '.').resolve()
    problems, checked = [], 0

    for src in doc_set(root):
        rel = src.relative_to(root)
        for target in LINK.findall(prose_of(src)):
            checked += 1
            if target.startswith(('http://', 'https://', 'mailto:')):
                continue
            if target.startswith('#'):
                if target[1:] not in headings_of(src):
                    problems.append(f"{rel}: self-anchor {target} not found")
                continue
            path_part, _, anchor = target.partition('#')
            if not path_part:
                continue
            dest = (src.parent / path_part).resolve()
            if not dest.exists():
                problems.append(f"{rel}: broken link -> {target}")
            elif anchor and dest.suffix == '.md' and anchor not in headings_of(dest):
                problems.append(f"{rel}: anchor #{anchor} not in {path_part}")

    print(f"  {checked} links across {len(doc_set(root))} pages")
    for problem in problems:
        print(f"  FAIL {problem}")
    return 1 if problems else 0


if __name__ == '__main__':
    sys.exit(main())
