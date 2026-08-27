# Contributing: branches and commits

Branch names and commit messages follow hard rules, for humans and coding agents
alike. One implementation backs every enforcement point — the conventions action in
[code.examples.ci](https://github.com/josnelihurt/code.examples.ci) — so the CI gate, the
optional local hooks and a manual check can never drift apart; this repository holds only
thin execs over the [code.examples.ci](https://github.com/josnelihurt/code.examples.ci)
submodule (`ci/` — `scripts/check-conventions.sh`, `.githooks/*`).
[AGENTS.md](../AGENTS.md) carries the agent-facing summary; this page is the full reference.

## Branch naming

Every branch pushed to the repository matches:

```text
^(feature|hotfix|chore|docs|ci|fix)/[a-z0-9][a-z0-9-]*[a-z0-9]$
```

A prefix from the table below, a slash, then a kebab-case name of at least two
characters (digits allowed, e.g. a tracking issue number: `feature/e2e-db-19`).

| Prefix | Use |
| ------ | --- |
| `feature/` | new capability |
| `hotfix/` | urgent fix on a broken behavior |
| `chore/` | tooling, dependencies, repo upkeep |
| `docs/` | documentation changes |
| `ci/` | build and pipeline changes |
| `fix/` | non-urgent bug fixes |

Exemptions: `main` itself, and `backup/…` branches — those are local-only
snapshots the [stacked-PR workflow](../AGENTS.md#big-changes-land-as-stacked-pull-requests)
never pushes.

## Commit messages

Every commit subject and every pull-request title matches:

```text
type(scope)!: lowercase imperative summary
```

- `type` — one of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`,
  `build`, `ci`, `chore`, `revert`
- `(scope)` — optional lowercase scope, e.g. `refactor(auth): …`
- `!` — optional breaking-change marker
- summary — imperative mood ("add", not "added"), starts with a lowercase
  letter or digit, no trailing period, whole line at most 72 characters

Why the PR title too: this repository squash-merges, so **the PR title becomes
the canonical commit on `main`**. GitHub appends ` (#N)` at merge time — that
suffix is legal only on the merged result (the push-side check allows it), never
in a PR title or an in-stack commit.

Good:

```text
feat: add branch and commit conventions check
refactor(auth): extract token validation
feat(api)!: rename the auth endpoints
```

Bad:

```text
update stuff                        — no type
feat: Add capital summary           — summary must start lowercase
fix: ends with period.              — no trailing period
docs: document the workflow           — no issue or pull-request trailers
```

## Enforcement

### CI

The `conventions` job in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)
runs on every pull request and every push to `main`, ungated by the path filters
that scope the heavier jobs — branch and commit rules describe the change
itself, not the files it touches. On a PR it checks the branch name, every
commit GitHub attributes to the PR (the pulls-API list — exactly the commits the
squash merge would collapse), and the PR title. Commits carrying a trailing
` (#N)` are skipped on the PR side: the server-side stack rebase materializes
already-merged lower layers on upper branches with the squash subject, and those
artifacts are the base branch's history, not this PR's contribution. On a push
to `main` it checks the new commits' subjects, validating with the ` (#N)`
suffix stripped.

The same workflow runs `secrets-hygiene` (also ungated) and the `changes`
path-detection job whose filters drive the gated jobs: build & test, lint,
CodeQL, BDD specs, full-stack e2e, contract drift, image pins, and `docs`
(links + code references) — the gate that runs `./scripts/verify-docs.sh` on
every PR touching a documentation page.

### Branch ruleset on `main`

A repository ruleset requires a pull request, a green `conventions` check, and
blocks force pushes and deletion before anything lands on `main`. It deliberately
does **not** require approvals, "up to date" branches, or a merge queue: the
[merge-me automation](../.github/workflows/merge-me.yml) merges with
`GITHUB_TOKEN`, which cannot bypass branch protection, and stacked layers are
rebased server-side when a lower layer merges. The merge-queue revisit is
tracked in [code.examples.net.quotes
issue #7](https://github.com/josnelihurt/code.examples.net.quotes/issues/7).

To recreate the ruleset after a repository reset:

```bash
gh api -X POST repos/{owner}/{repo}/rulesets --input - <<'JSON'
{
  "name": "conventions",
  "target": "branch",
  "enforcement": "active",
  "conditions": { "ref_name": { "include": ["refs/heads/main"], "exclude": [] } },
  "rules": [
    { "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 0,
        "dismiss_stale_reviews_on_push": false,
        "require_code_owner_review": false,
        "require_last_push_approval": false,
        "required_review_thread_resolution": false
      } },
    { "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": false,
        "required_status_checks": [
          { "context": "conventions (branch names + commit messages)" } ] } },
    { "type": "deletion" },
    { "type": "non_fast_forward" }
  ]
}
JSON
```

## Local hooks (optional)

For feedback before the push, opt into the git hooks:

```bash
./scripts/setup-git-hooks.sh   # enable  (git config core.hooksPath .githooks)
git config --unset core.hooksPath   # undo
```

- `commit-msg` validates the subject of the commit being created.
- `pre-push` validates the branch name and every commit not yet on `origin/main`
  (the whole delta, so stacked branches are covered); `main`, `backup/*` and
  tags are exempt.

Both hooks exec canonical scripts from the `ci/` submodule on disk
(`ci/conventions/scripts/commit-msg` and `pre-push`), so hook logic is never
duplicated in consuming repositories and hooks run offline. The submodule pin
moves by pull request — like the frontend pin — and a pin move forces the full
CI matrix through the `workflow` path filter. Clone with
`git clone --recurse-submodules`, or run `git submodule update --init` after a
plain clone; `scripts/setup-git-hooks.sh` initializes the submodule for you.

Pure git configuration — no package-manager lifecycle, matching the
package-manager posture (in
[code.examples.frontend.quotes](https://github.com/josnelihurt/code.examples.frontend.quotes)). CI enforces the same
rules regardless of whether the hooks are installed.

## The checker

```text
scripts/check-conventions.sh --branch <name>          # branch naming rule
scripts/check-conventions.sh --range <a>..<b>         # commit subjects in range
scripts/check-conventions.sh --title <text>           # PR title rule
                       [--allow-pr-number]            # tolerate one trailing " (#N)"
```

Modes combine; exit codes: `0` clean, `1` violations (all reported), `2` usage
error.

## Contracts toolchain image

The buf + protoc-gen-openapiv2 pair behind the hermetic OpenAPI freeze is not
downloaded per build anymore: it lives in the prebuilt toolchain image
`ghcr.io/josnelihurt/code.examples.go.quotes/toolchain/contracts`, built from
`contracts/docker-build-base/Dockerfile` and published by the
`.github/workflows/toolchain.yml` workflow (multi-arch, gha-cached, immutable
tag). `Dockerfile.build` consumes it `FROM`-pinned by digest, so the checksum
verification that used to run on every freeze ran once, at publish time.

Bumping the pins is a four-step, one-PR change (still paired with the .NET
repository's own bump, re-freezing in the same change — see ADR 0003's
addendum):

1. Edit the pair in both `contracts/docker-build-base/VERSION` (the tag,
   `<buf>-<openapiv2>`) and the `ARG`s in `contracts/docker-build-base/Dockerfile`.
2. Publish: merge to `main` or `workflow_dispatch` the toolchain workflow; its
   job summary prints the pushed digest. (PR runs only compile-validate the
   image, and only when `contracts/docker-build-base/**` changed.)
3. Pin that digest: append `@sha256:…` to the `FROM` line of `Dockerfile.build`.
4. Re-freeze the document with `scripts/update-contracts.sh` and commit it.

## Discarded errors

Writing an HTTP response body can fail, and when it does there is nothing the handler can
do: the status line is already on the wire, the client is usually gone, and there is no
second response to send. So the response writers here discard that error deliberately —
`_ = json.NewEncoder(w).Encode(...)` in `problemjson` and the auth transport,
`_, _ = w.Write(...)` in the health endpoints and the v3 documentation routes.

**That is the blanket rule: a failed write to an HTTP response after the status has been
sent is discarded, and needs no per-site comment.** Stating it once here keeps ten
near-identical comments out of the code. `errcheck` is satisfied by the explicit `_ =`,
which is the point of writing it rather than ignoring the value silently.

A discard *outside* that rule does need its rationale at the call site, because a reader
cannot tell it apart from an oversight. The three that exist say why in one trailing
clause each — `internal/quotes/infrastructure/migrate.go`,
`internal/platform/config/config.go` and `internal/quotes/domain/quote.go`. Follow that
form rather than adding a new blanket exemption.
