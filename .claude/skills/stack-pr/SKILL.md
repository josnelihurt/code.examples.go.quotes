---
name: stack-pr
description: Land an ordered set of issues as a stacked pull request chain in which every level compiles, tests and lints independently — split, cut, verify per level, push, open bottom-up, register the stack, hand off to merge-me, then close the issues and confirm CI is green. Use when a triaged stack plan is ready to implement.
---

# Stacked PR

Executes `AGENTS.md` § "Big changes land as stacked pull requests". That section is canonical;
this skill is the runnable form of it. Read it before starting — if the two ever disagree,
`AGENTS.md` wins and this file is the bug.

## The invariant

**Every level compiles, passes lint, and passes every CI gate independently.** If an intermediate
level would be red, the split is wrong — redo the split. This is the whole point; a stack whose
middle is red is one big PR wearing a costume.

## Procedure

### 1. Build and verify the end state first

`make build`, `make test`, `make lint` green, plus the suite scripts the layers under change
carry (`./scripts/bdd.sh`, `./scripts/e2e.sh`, `./scripts/update-contracts.sh`). Then snapshot:

```
git checkout -b backup/<name> && git add -A && git commit -m "chore: snapshot before splitting"
```

**Never push a backup branch.** Reset the working branch clean before splitting.

Knowing the end state is what makes the split honest — you are dividing a verified result, not
hoping the pieces converge.

### 2. Choose the split by decision, bottom to top

Schema and foundations first · adapters beside the old implementation · no-op plumbing
(containers, config, CI steps) as layers a later PR makes load-bearing · then the behavior switch
· then pure deletion of the old path · docs last.

One decision per layer. Where a clean seam is impossible, leave a **temporary bridge** (a
coexisting wiring path) and remove it in the deletion layer — a bridge you never remove is a
finding against the stack.

### 3. Cut branches in order

Each from the previous one's head. `(feature|hotfix|chore|docs|ci|fix)/kebab-case-<issue>`.
Pull file subsets from the backup with `git checkout backup/<name> -- <paths>`; **hunk-split**
files two layers share (`ci.yml`, `docker-compose.yaml`) rather than duplicating them. Author
intermediate states explicitly — do not hope the final files compile mid-stack.

### 4. Implement and revise each layer

Per layer: spawn `stack-implementer` with a **self-contained** prompt (the layer spec, the branch,
the parent, the gates), then spawn `stack-revisor` on the result. On FAIL, re-prompt an
implementer for the blocking findings and re-revise with a fresh revisor. **At most three
implement→revise iterations per layer** — if the third still fails, stop and report rather than
iterating further.

Verify **at the load-bearing levels, not only the tip**: at each level that changes behavior, run
the suites it could break, at that level. Config-only layers need a build check.

Move CI path filters with the files they gate, in the same layer (repo-rules.md §R8).

### 5. Push and open bottom-up

Push each branch; open each PR with base = the branch below (the bottom one → `main`). Body:

- **What** — one paragraph.
- **Stack** — part N of M, previous and next links.
- **Review pointers** — the three or four things to actually look at.
- **Evidence** — which suites ran green *at this level*.

Then register the chain: `gh extension install github/gh-stack` once,
`gh stack link <bottom-pr> … <top-pr>`, and `gh stack link <stack-number> <new-pr>` to append.

### 6. Hand off the merge

Label **only the top-layer PR** `merge-me`. One label lands the whole chain, bottom-up and
atomically; GitHub rebases and retargets the layers above as each merges.

**Never**: merge by hand · label several layers (concurrent merges race —
code.examples.net.quotes#10) · rebase, force-push or delete a mid-stack branch · edit a PR base
by hand.

### 7. Close out

- `gh pr checks` green on every layer before labeling.
- After the chain lands: `gh issue close <n> --comment "Landed in #<pr>."` for each issue.
- `gh run list --limit 5` on `main` — confirm green.
- `git diff main..backup/<name>` — the only acceptable delta is what was deliberately
  reorganized. Then delete the backup branch locally.

## Reporting

Report the chain as a table: layer · branch · PR · issue · gates run · verdict. Then the merge
state and the final CI result. Never report a gate as green that you did not run, and never
report the stack as landed before `gh run list` says so.
