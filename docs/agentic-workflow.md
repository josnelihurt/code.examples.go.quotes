# Agentic workflow

This is how the repository was actually built: not one long-lived coding session, but a stack of
pull requests — one decision per layer, every level independently green — produced by three
cooperating agent roles. Humans review every layer; [AGENTS.md](../AGENTS.md) carries the working
agreements, [contributing.md](contributing.md) the conventions enforcement, and
[architecture-decisions.md](architecture-decisions.md) the evaluations the layers implement.

## Roles

- **Orchestrator** — supervises and never writes feature code. It owns the todo list and the stack
  plan, cuts each branch from the previous layer's head, writes the per-layer prompts, pushes,
  opens the pull requests, registers the stack (`gh stack link`) and hands off merges. Its context
  holds plans and verdicts, not file contents.
- **Implementer** — a fresh-context sub-agent spawned per layer. Its prompt is self-contained:
  the layer specification, the repository conventions, the wire semantics the layer must not
  move, and the gates to run before claiming done. It lands exactly one commit —
  `type: lowercase imperative subject` — and reports what changed and which gates ran green.
- **Revisor** — a fresh-context sub-agent that validates the finished layer against the checklist
  below and returns a PASS/FAIL verdict with findings graded **blocking**, **minor** or **note**.
  It never fixes anything: findings travel back to an implementer.

## Loop per layer

1. The orchestrator cuts the branch for layer N from the head of layer N−1 and hands the
   implementer the self-contained prompt.
2. The implementer lands its single commit and reports the result with evidence.
3. The revisor reviews the layer — the diff against the parent branch, plus the checklist — and
   returns PASS or FAIL with graded findings.
4. On FAIL the orchestrator re-prompts an implementer to fix the blocking findings, then a fresh
   revisor re-revises. At most three implement→revise iterations per layer; if the third revision
   still fails, stop and report instead of iterating further.
5. On PASS the orchestrator pushes the branch, opens the PR (body: What · Stack · Review
   pointers · Evidence at this level), links it into the GitHub stack, and verifies
   `gh pr checks` is green *at that level* before starting layer N+1.

## Revisor checklist

**A. Conventions.** The branch name matches
`^(feature|hotfix|chore|docs|ci|fix)/[a-z0-9][a-z0-9-]*[a-z0-9]$`; the branch carries exactly one
commit; its subject — and the PR title — follow `type(scope)!: lowercase imperative summary`
(allowed types, ≤72 characters, no trailing period); `./scripts/check-conventions.sh --branch
<name> --title "<title>" --range <base>..HEAD` exits 0.

**B. Layer independence.** Builds, tests and lint pass *at this level's commit*, not only at the
stack tip; the diff against the parent branch touches only files this layer owns; a layer that
adds a CI job or a load-bearing file extends the `changes` path filters in the same PR.

**C. Wire semantics.** The pinned v3 contract still holds: the error envelope
(`{"code":N,"message":…}` with `"details":[]` present), the gRPC→HTTP status mapping, paging
defaults (1/20) and bounds, the `X-Correlation-Id` echo, the 401 problem+json body with
`WWW-Authenticate`, and create answering 200 with no `Location` header. The wire tests and the
specs are what prove it; where a layer changes one of these deliberately, the ADR that records
the decision changes in the same layer.

**D. Hygiene.** No secrets; no stray debug output or dead code; documentation updated alongside
the change; the PR body carries What / Stack / Review pointers / Evidence-at-this-level.

Blocking findings fail the layer; minor findings are fixed in the same iteration when cheap,
otherwise recorded for a later layer; notes are informational.

## Merge handoff

When the revisor passes the top of the stack, the orchestrator labels **only the top-layer PR**
with `merge-me`. The [merge-me workflow](../.github/workflows/merge-me.yml) then merges the chain
bottom-up — atomically, everything below included — and GitHub rebases and retargets the layers
above as each PR lands. Never label several layers: concurrent labels start merges that race.
Never merge by hand: the workflow exists so merges are uniform and branch protection stays
intact. Do not rebase, force-push or delete mid-stack branches.

## Context economy

Sub-agents do the file work because reading and editing dozens of files is what burns a context
window. The orchestrator keeps only the layer specs, the revisors' verdicts and the stack state;
each implementer and revisor starts fresh, reads exactly what its prompt names, and returns a
bounded result. A stack of many layers stays supervisable in one session because no single
context ever carried all the layers' worth of code at once.
