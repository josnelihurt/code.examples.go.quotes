# AGENTS.md

Working agreements for coding agents in this repository. Humans reviewing the results are
the audience that matters — every rule below exists to keep what lands reviewable.

## Hard rules: branch names and commit messages

Enforced by the `conventions` CI job on every PR and by a ruleset on `main` — a violating
PR cannot merge. Full reference: `docs/contributing.md`.

- **Branch names**: `feature/`, `hotfix/`, `chore/`, `docs/`, `ci/` or `fix/` + a kebab-case
  name (issue-number suffix encouraged: `feature/e2e-db-19`). Local-only `backup/…`
  snapshots are exempt because they are never pushed.
- **Commit subjects**: `type: lowercase imperative summary`, ≤72 characters, types
  `feat fix docs style refactor perf test build ci chore revert` (optional `(scope)` and
  `!`). **PR titles follow the same rule** — squash merges make the title the canonical
  commit on `main`; the `(#N)` suffix GitHub appends at merge time is the only difference.
- Local fast feedback is opt-in: `./scripts/setup-git-hooks.sh`. CI enforces regardless.

## Big changes land as stacked pull requests

Never open one large PR. Decompose the change into an ordered chain in which **every
level compiles, passes lint, and passes every CI gate independently**. If an
intermediate level would be red, the split is wrong — redo the split.

The recipe (proven in the .NET seed on its two-layer PostgreSQL-catalog stack — catalog
storage beneath the repository port):

1. **Build and verify the end state first** — `go build ./...`, `go test ./...` and
   `golangci-lint run` green (plus the suite scripts the layers under change carry:
   `scripts/bdd.sh`, `scripts/e2e.sh`, `scripts/update-contracts.sh`). Then snapshot
   uncommitted work to a local backup branch
   (`git checkout -b backup/… && git add -A && git commit`) before splitting, and reset
   the working branch clean. Never push the backup branch.
2. **Choose the split by decision**, bottom to top: schema/foundations first; adapters
   beside the old implementation; no-op plumbing (containers, config, CI steps) as layers
   that a later PR makes load-bearing; then the behavior switch; then pure deletion of
   the old path; docs last. Where a clean seam is impossible, leave a **temporary
   bridge** (e.g. a coexisting wiring path) and remove it in the deletion layer.
3. **Cut branches in order**, each from the previous one's head. Pull file subsets from
   the backup (`git checkout backup/… -- <paths>`); hunk-split files shared by two PRs
   (`ci.yml`, `compose.yaml`) rather than duplicating them; author intermediate states
   explicitly instead of hoping the final files compile mid-stack.
4. **Verify at the load-bearing levels, not only the tip.** At each level that changes
   behavior, run the suites it could break — at that level — before moving on
   (`go build ./...`, `go test ./...`, `golangci-lint run`, and the suite scripts that
   layer carries). Config-only layers need a build check. Finish with the full sweep at
   the tip, then diff the tip against the backup: the only acceptable delta is what was
   deliberately reorganized.
5. **One commit per branch**, message as `type: lowercase imperative` with the enforced
   type set (`feat fix docs style refactor perf test build ci chore revert` — see the
   hard rules above) — the same convention as the repository history.
6. **PR body** = **What** (one paragraph) · **Stack** (part N of M, prev + next links) ·
   **Review pointers** (the three or four things to actually look at) · **Evidence**
   (which suites ran green *at this level*).
7. **Push the branches, open the PRs bottom-up** with each base = the branch below
   (the bottom one → `main`), then register the chain as a GitHub stack:
   `gh extension install github/gh-stack` once, `gh stack link <bottom-pr> … <top-pr>`
   for existing PRs, and `gh stack link <stack-number> <new-pr>` to append later layers.
8. **After registration, merging is bottom-up and automatic** — GitHub rebases and
   retargets the layers above as each PR merges. Do not rebase, force-push, or delete
   mid-stack branches, and do not edit PR bases by hand. Never merge by hand: labeling a
   reviewed PR `merge-me` hands it to the merge-me workflow, which merges green PRs
   itself (stack layers atomically, everything below included) — see the thin
   [.github/workflows/merge-me.yml](.github/workflows/merge-me.yml) wrapper and the shared
   [code.examples.ci](https://github.com/josnelihurt/code.examples.ci) `merge-me` action
   it pins. Label the top layer only: one label lands the whole chain, and labeling
   several layers starts concurrent merges that race (code.examples.net.quotes#10).

What matters most: every intermediate level green, per-level evidence in the PR bodies,
and a tip that matches the independently verified end state.

## CI runs only the jobs a change can affect

`ci` gates every job (except path detection, secrets hygiene and conventions) on the
areas the PR touches: a markdown-only change runs neither the backend nor the e2e
matrix, and a backend-only change skips the e2e job. The gates live in the `changes` job
of `.github/workflows/ci.yml` — a PR that adds a job or a load-bearing file extends the
filters in the same PR. The `ci:full-build` PR label forces the full matrix; pushes to
`main` always run it, and so does any change under `.github/workflows/**`.

At this scaffold layer the matrix is `conventions` and `secrets-hygiene` only (both
ungated); the gated jobs — build & test, lint, CodeQL, BDD specs, full-stack e2e,
contract drift and image pins — land with the Go-module and stack layers that make them
load-bearing, each adding its decide output in the same PR.

Skips happen at the job level on purpose: skipped check runs still satisfy the
branch-protection checks, and the workflow still completes success so merge-me's
ci-completion trigger keeps firing. A workflow-level `paths:` filter would break both.
Verify a new gate with a throwaway probe PR (docs-only, backend-only, …) before
labeling `merge-me`.
