# Generic Go review rubric

Portable Go best practice. Nothing here is specific to this repository — the repo-specific
rules live in [repo-rules.md](repo-rules.md). Sources are the Go team's own guidance
(Effective Go, the Go Code Review Comments wiki, the standard library's own style) plus the
community `golang-standards/project-layout`, which is explicitly **not** a Go team standard and
is cited as such below.

Every finding must cite a rule number from this file or from repo-rules.md. A observation you
cannot map to a rule is a **question**, not a finding.

---

## 1. Project layout

1.1 **`cmd/<name>/` holds one `package main` per binary**, and holds as little else as possible.
Composition logic that grows past a few hundred lines wants to move into an internal package the
binary calls — `main` should read as a wiring list.

1.2 **`internal/` is the default home for code the module does not publish.** Go enforces
`internal/` at the compiler level; a package outside it is part of your public API forever,
whether you meant that or not. A package that only this module's own binaries import, but which
sits outside `internal/`, is a finding: it publishes an implementation detail.

1.3 **No Go packages under documentation or asset directories.** `docs/`, `examples/`, `web/`,
`assets/` are read by humans and build tooling, not compiled into the product. An importable Go
package under one of them is a layout finding. *Exception worth checking before reporting:*
`//go:embed` cannot reach across a parent directory, so an embed file **must** sit beside the
files it embeds. When the embedded asset legitimately lives under `docs/`, the embed file has
nowhere else to go — report the tension and both options (move the asset, or accept the Go file
where it is and document why), never just the ideal.

1.4 **IDL and schema live in one named directory** (`api/` per project-layout, or `contracts/`,
or `proto/`) — not scattered beside the code they generate. Generated output may live beside its
consumer; the source of truth may not be duplicated.

1.5 **Cross-cutting test suites live in `test/`** per project-layout (community convention, not a
Go team standard — a repo using `tests/` is *not wrong*, and moving it is rarely worth the
churn). Unit tests always stay beside the code they test.

1.6 **Deviation from a layout convention is fine when it is documented.** An undocumented
deviation costs every new reader; a deviation with a one-paragraph rationale in the README or an
ADR costs nothing. Prefer recommending the documentation over recommending the move whenever the
move's blast radius is wide.

---

## 2. Package naming

2.1 **The package name matches the directory's base name.** When they differ, every importer
either aliases the import or silently uses a name the path did not predict. Both are friction.
(`package main` in `cmd/<name>/` is the standard exception.)

2.2 **Names are short, lowercase, singular, no underscores, no camelCase.** `bytes`, `http`,
`strconv` — not `byte_utils`, `httpHelpers`, `strconvs`.

2.3 **No `util`, `common`, `helpers`, `misc`, `base`, `shared`.** A package named for what it is
*not* accumulates everything. If you cannot name the package for what it provides, the boundary
is wrong.

2.4 **The name is not repeated in its exported identifiers.** `http.Server`, not `http.HTTPServer`;
`bytes.Buffer`, not `bytes.BytesBuffer`. Callers read `pkg.Name`, so the package name is already
half the sentence.

2.5 **A package that most call sites must alias is misnamed.** Widespread aliasing
(`import foo2 "…/foo"`, `import authinfra "…/auth/infrastructure"`) is direct evidence that the
path and the name disagree, or that two packages collide on a generic name.

2.6 **Generic layer names repeated across bounded contexts collide.** Five packages named
`infrastructure` in one module means five aliases at every composition root. Real, but usually
low urgency — report as a note unless the aliasing is actively causing mistakes.

---

## 3. Errors

3.1 **Wrap with `%w` and add context that the caller does not already have.**
`fmt.Errorf("opening the catalog: %w", err)`. Never `fmt.Errorf("error: %v", err)` — that
destroys the chain and adds nothing.

3.2 **Do not stutter.** `failed to X: failed to Y: failed to Z` — each layer adds its own noun,
not another "failed to".

3.3 **Prefer a sentinel or typed error over `(nil, nil)`.** Returning a nil value with a nil
error for "not found" pushes the absence check onto every caller and is invisible in the
signature. `ErrNotFound` (checked with `errors.Is`) or a typed error (`errors.As`) states it.
*A documented port contract that returns `(nil, nil)` deliberately is a defensible choice* —
report it as a note with the idiom named, not as a defect, when the interface documents it.

3.4 **Never return a meaningful-looking value beside a non-nil error.** Callers who check the
error first are fine; the one who does not gets a value that reads as real. Return the zero
value, or add an explicit `Unknown`/`Invalid` member to the result type.

3.5 **Compare errors with `errors.Is` / `errors.As`, never `==` or string matching**, once any
layer wraps. (`errorlint` catches this.)

3.6 **A discarded error needs a comment.** `_ = w.Write(...)` on an HTTP response is a normal,
correct choice — the client is gone and there is nothing to do. Say that in a comment, once,
where it happens. An uncommented `_ =` is indistinguishable from an oversight.

3.7 **Errors are values, not control flow.** `panic` in library code is a finding; recovering
from a panic to implement ordinary behavior is a finding.

---

## 4. Context

4.1 **`ctx context.Context` is the first parameter**, always named `ctx`, never stored in a
struct field. (`contextcheck` and `containedctx` catch these.)

4.2 **`context.TODO()` does not ship.** In a shipped path it means someone deferred a decision
and forgot. `context.Background()` at a genuine root (a `main`, a test) is correct.

4.3 **A blocking call reachable from a request takes the request's context** — database calls,
HTTP calls, lock acquisitions with timeouts. A blocking call that ignores cancellation makes
shutdown and timeouts a lie.

4.4 **Context carries request scope, not optional parameters.** Correlation ids and deadlines,
yes. Configuration and dependencies, no.

---

## 5. Interfaces

5.1 **The consumer defines the interface, not the implementer.** Go interfaces are satisfied
structurally, so the package that *needs* the behavior declares the small interface it needs.
An interface declared beside its single implementation, for that implementation's benefit, is
usually premature.

5.2 **Interfaces are small.** One to three methods is the norm; the standard library's most
reused interfaces have one. A large interface has one implementation by construction.

5.3 **Accept interfaces, return structs.** Returning an interface hides the concrete type's
other methods and forces callers through the narrow view you guessed at.

5.4 **`var _ Iface = (*Impl)(nil)` is the right way to assert satisfaction** at compile time —
cheap, and it fails at build rather than at wiring.

5.5 **No interface with exactly one implementation and no test double using it**, unless it is a
declared port at an architectural boundary. Otherwise it is indirection with no reader benefit.

---

## 6. Concurrency

6.1 **Every goroutine has a defined exit.** If you cannot say what stops it, it leaks.

6.2 **No naked `go func()` in a request path** — it outlives the request, escapes the request
context, and its panic kills the process.

6.3 **Channel buffer sizes are deliberate.** A buffered channel exists to decouple a specific
producer/consumer rate; `make(chan T, 1)` to avoid a goroutine leak on an abandoned send is the
common correct case and deserves the comment saying so.

6.4 **Shutdown is bounded.** A graceful stop with no timeout is a hang under load. Graceful,
then a hard stop after a stated budget.

6.5 **Shared mutable state is guarded, and the guard is documented** — which mutex covers which
fields, stated where the fields are declared.

6.6 **The race detector runs in CI** (`go test -race`). Concurrency review without it is guessing.

---

## 7. API and wire safety

7.1 **Narrowing integer conversions are truncation bugs until proven otherwise.**
`int32(x)` where `x` is an `int` silently wraps above 2³¹−1. Either validate the bound before
converting, or use a type that cannot overflow. (`gosec` G115.) This matters most where the
value came from a request.

7.2 **Every value crossing the wire is validated at the boundary**, not deep inside. Length
bounds, ranges, enum membership.

7.3 **JSON field names and presence semantics are API contract.** A pointer field means
"distinguishable absence"; changing it to a value silently changes the wire. Struct tags are
reviewed as carefully as the fields.

7.4 **HTTP handlers set status before writing the body**, and set `Content-Type` explicitly.

7.5 **`defer resp.Body.Close()` on every HTTP response you receive** (`bodyclose`), and every
`rows.Close()` on every query.

---

## 8. Testing

8.1 **Table-driven tests are the default shape**, with a named `tests` slice and `t.Run(name, …)`
so a failure names itself.

8.2 **`t.Parallel()` wherever it is safe**, and never where the test mutates process state.

8.3 **A package whose only files are `_test.go` never compiles under `go build ./...`.** It is
legal and sometimes deliberate (an integration suite gated behind a tag or a runner script), but
it means ordinary builds do not catch drift there — the suite only fails when someone runs it.
Report it as a note with that consequence stated.

8.4 **Test helpers call `t.Helper()`**, so failures point at the assertion, not the helper.

8.5 **Cleanup via `t.Cleanup`**, not `defer` in a helper that returns before the test ends.

8.6 **Every bug fix gets a pinning test** at the lowest level that fails without the fix.

8.7 **Test code is product code** — same lint, same review, same naming.

---

## 9. Documentation and comments

9.1 **Every exported identifier has a doc comment starting with its name.** `// Server serves…`,
not `// This struct is the server…`.

9.2 **Each package has one package comment**, on one file, starting `// Package <name> …`.

9.3 **Comments explain *why*, not *what*.** The code already says what. A comment restating the
next line is noise; a comment naming the constraint that forced the line is the reason review is
possible at all.

9.4 **A comment that contradicts the code is worse than no comment.** Check them against each
other — a stale comment is a real finding.

---

## 10. Tooling

10.1 **`gofmt` / `gofumpt` is not negotiable** and should be enforced, not requested.

10.2 **A linter set that omits the security and error linters is incomplete.** Baseline worth
having: `errcheck govet staticcheck unused revive gocritic` plus `errorlint` (wrapping),
`gosec` (G115 truncation, and the rest), `bodyclose` (leaked response bodies), and — where the
project uses them — `sloglint` (structured-logging consistency) and `testifylint`.

10.3 **`nolint` needs a reason.** `//nolint:revive // GetQuoteById is the rpc's name in the
generated contract` is a good suppression; a bare `//nolint` is a finding. (`nolintlint`
enforces this.)

10.4 **Generated code is never hand-edited.** It is regenerated by a pinned tool, and the pin
lives in the repository. A hand-edit to a `*.pb.go` or `*.sql.go` is blocking.

10.5 **CI path filters move with the files they gate.** A change that relocates a load-bearing
directory and leaves its filter pointing at the old path is red or, worse, silently green.
