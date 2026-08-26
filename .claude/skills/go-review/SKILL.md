---
name: go-review
description: Review this repository's Go code against generic Go best practices and the repository's own rules — project layout, package naming, errors, context, interfaces, concurrency, wire safety, tests — and produce a ranked findings table for triage. Use for a whole-repository layout pass, or scoped to a diff or a package. Read-only; produces findings, never changes.
---

# Go review

Produces a **ranked findings table**. It changes nothing and opens nothing — `triage-findings`
turns the table into issues, `stack-pr` turns issues into merged PRs.

## Scope

Default is the whole tree (a layout pass). Scope it by argument when given one: a diff
(`main..HEAD`), a package (`./internal/quotes/...`), or a PR number.

## Procedure

1. **Baseline first.** Run and record:

   ```
   go build ./...
   go vet ./...
   golangci-lint run
   gofmt -l .
   ```

   Findings the toolchain already reports are toolchain findings — cite the tool rather than
   presenting them as discoveries. A green baseline tells you the remaining findings are design
   and layout, which is the interesting case.

2. **Enumerate before reasoning.** `go list ./...` for packages;
   `find . -name '*.go' -not -path './frontend/*'` for files. For each package, compare the
   declared package name against its directory base name — §2.1 of the rubric is the cheapest
   real finding in most repositories and it is mechanical:

   ```
   for f in $(go list -f '{{.Dir}}/{{.Name}}' ./...); do echo "$f"; done
   ```

3. **Delegate the review to the `go-reviewer` agent.** Hand it the scope and the two rubric
   paths. It is read-only by construction. For a large tree, one agent is enough — do not fan
   out across packages; the layout findings are cross-package by nature and a split view misses
   them.

4. **Measure every blast radius before ranking.** A finding proposing a move or rename is not
   ready until every referencing file is named. Grep `*.go`, `*.yml`, `*.yaml`, `*.sh`,
   `Dockerfile*`, `Makefile`, `*.md`, **and the `ci/` and `frontend/` submodules** — a path that
   another repository hardcodes has a cost this repository cannot pay alone (repo-rules.md §R5).

5. **Rank by severity × blast radius**, cheapest-correct first, and emit the table.

## Output

A table — one row per finding: **id · severity · finding · file:line · rule · minimal fix ·
blast radius**. Ids are stable and prefixed by category (`L#` layout, `I#` idiom, `T#` tests,
`C#` CI). Then:

- **Conflicts** — findings whose fixes collide with each other or with a cross-repo convention,
  each with both options and their real costs, and a recommendation.
- **Notes** — judgment calls and questions. No action implied.
- **Baseline** — what step 1 reported.

Severity is **blocking** (breaks a CI-enforced rule, or a real defect), **minor** (breaks a rule
nothing enforces yet), or **note**.

## Rules

- The rubrics are `references/go-rubric.md` (generic, portable) and `references/repo-rules.md`
  (this repository). **Read both** — never review from memory of what Go style usually is.
- Every finding cites a rule. One that cannot is a question, reported under Notes.
- **Never recommend a move purely for layout purity when a cross-repo convention holds the
  path.** Report both options and their costs; the human decides at triage.
- Do not create issues, branches, or PRs here. Ranking and reporting is where this skill ends.
