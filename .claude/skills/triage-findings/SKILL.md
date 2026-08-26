---
name: triage-findings
description: The human gate between a review and GitHub. Takes a ranked findings table, presents it for the user to select and order, then opens GitHub issues for the chosen findings only and records the approved order as a stack plan. Use after go-review and before stack-pr.
---

# Triage findings

The gate where **the user decides**. A finding becomes an issue only because the user picked it.

## Hard rule

**Never create an issue for a finding the user did not explicitly select.** Not "it was obviously
right", not "it was cheap", not "it was already half-done in another layer". A review that files
its own findings is not a gate — it is a review with write access. If the selection is ambiguous,
ask again rather than inferring generously.

## Procedure

1. **Present the table.** Show the ranked findings as the reviewer produced them — id, severity,
   finding, file:line, minimal fix, blast radius. Surface **Conflicts** prominently: a finding
   with two viable options needs the user choosing an option, not just choosing the finding.

2. **Ask with `AskUserQuestion`.** Use `multiSelect: true` for "which findings become issues".
   Put the recommendation first and mark it `(Recommended)`. Where a finding has competing
   options (the cross-repo conflicts especially), ask that as its own single-select question with
   the costs in each option's description — the user cannot weigh a tradeoff you summarized away.

3. **Confirm the order** if the user's selection does not imply one. Order matters: it becomes
   the stack's bottom-to-top sequence, and `stack-pr` requires foundations first.

4. **Create the chosen issues.** `gh issue create` per selected finding:
   - **Title** — `type: lowercase imperative summary`, ≤72 chars, same convention as commits
     (repo-rules.md §R6), because the PR title that closes it must follow the rule anyway.
   - **Body** — *What* (the finding, with `file:line`) · *Why* (the rule it breaks, cited) ·
     *Minimal fix* · *Blast radius* (every referencing file) · *Gates* (which must run green).
   - **Labels** — reuse what the repository already has (`enhancement`, `documentation`, `bug`).
     Do not invent labels without asking.
   - Include `Part N of M` when the findings form an ordered stack.

5. **Record the stack plan.** Report the issue numbers in the approved bottom-to-top order. That
   list is the input `stack-pr` consumes.

## Verify before reporting

`gh issue list --state open` — the created issues exist, and **nothing exists that was not
selected**. Report the plan as a numbered list with the issue numbers and titles.

## What this skill does not do

It does not cut branches, write code, or open PRs. It ends at "here are the issues, in this
order". Findings the user declined are simply dropped — do not record them as deferred work or
re-raise them on the next run unless asked.
