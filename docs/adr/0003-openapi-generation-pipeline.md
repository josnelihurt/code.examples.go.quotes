# ADR 0003 — OpenAPI generation pipeline: hermetic, pinned to the .NET tool versions

* Status: accepted · Date: 2026-08-25*

## Context

Scope: how `docs/openapi/quotes-v3.openapi.json` (Swagger 2.0) is generated,
frozen, and kept honest in `josnelihurt/code.examples.go.quotes`.

## Decision

Mirror the .NET repo's hermetic freeze exactly: a `Dockerfile.build` `contracts` stage that runs
**buf v1.50.0** + **protoc-gen-openapiv2 v2.30.0** — the *same pins* the .NET `Dockerfile.build`
uses — over the same `quotes_v3.proto` plus the same vendored dependencies. Identical binaries +
identical inputs ⇒ near-byte-identical Swagger 2.0 output: that is the whole parity argument, and
it is why the versions are frozen rather than tracked to latest (exact versions under
[Pins](#pins)).

## Generation flow

1. `buf generate --path quotes_v3.proto` from the contracts directory (buf v2 `buf.yaml` +
   `buf.gen.yaml`, local plugin strategy).
2. The openapiv2 plugin emits `quotes_v3.swagger.json` (Swagger 2.0, driven by the proto's leading
   comments, `google.api.http` rules and `openapiv2_swagger` options — Bearer securityDefinitions,
   title `Quotes.Api | v3`).
3. Copy to the scratch `contracts` stage output as `quotes-v3.openapi.json`, frozen at
   `docs/openapi/quotes-v3.openapi.json`, committed.
4. The runtime embeds and serves those exact bytes at `/openapi/v3.json`
   ([ADR 0002](0002-v3-transport-grpc-gateway.md), item g).

`go_package` differs from the .NET config on purpose (real module path vs `example.invalid/unused`);
openapiv2 never copies that option into the Swagger document, so output parity is unaffected — the
one deliberate descriptor-level difference, and a non-observable one.

## Vendored proto dependencies

Committed in-repo exactly as the .NET repo does under `V3/Contracts/`:

- `google/api/annotations.proto`, `google/api/http.proto`
- `protoc-gen-openapiv2/options/annotations.proto`, `protoc-gen-openapiv2/options/openapiv2.proto`

Vendoring (vs `buf` remote dependency resolution) keeps the build offline-hermetic and pins the
transitive descriptor bytes that feed the generator — a remote `googleapis` update cannot churn the
frozen document.

## Drift protection

- **CI `contract-drift` job** — mirrors the .NET workflow: `docker build -f Dockerfile.build
  --target contracts -o type=local,dest=out .` then `diff -u docs/openapi/quotes-v3.openapi.json
  out/quotes-v3.openapi.json`. Any change to the proto, the vendored protos, the plugins or the
  Dockerfile that alters output fails the build until the frozen file is re-committed.
- **`scripts/update-contracts.sh`** — the local re-freeze path (`podman`/`docker` build + copy out
  of the scratch stage, namespaced image tag), to be run deliberately when the contract changes;
  review the diff before committing.

## Alternatives

- **buf remote plugins** (`buf.build/protocolbuffers/...`, `buf.build/grpc-gateway/...`) — resolve
  to whatever the registry builds today; version drift between two repos' runs breaks the
  byte-parity guarantee the freeze exists for, and the build stops being offline-hermetic.
- **OpenAPI v3 generators** (protoc-gen-openapi v3, googleapis/gnostic paths) — no maintained
  generator emits OpenAPI 3 from `google.api.http` rules (the .NET contract comment says as much);
  the .NET parity argument pins openapiv2/Swagger 2.0, and Swagger 2.0 is sufficient for the Scalar
  page and the frontend type sync.

## Consequences

+ The frozen document is reproducible bit-for-bit from either repository with the same two pins;
  contract drift is a CI failure, not a review-time surprise.
+ The proto remains the single contract of record; docs never hand-edited.
− buf v1.50.0 ages (Jan 2025); bumping it changes output whitespace/ordering at times, so both
  repositories must move the pin **together**, in paired PRs, and re-freeze in the same change.
− A Docker/podman build for a JSON file is heavyweight — accepted, because hermetic beats light
  when the artifact is a contract.

## .NET mapping

| .NET repo (origin/main) | Go port |
|---|---|
| `V3/Contracts` buf configs | buf v2 `buf.yaml` + `buf.gen.yaml` (local plugin strategy) |
| `Dockerfile.build` `contracts` stage (buf + protoc-gen-openapiv2) | same stage, same pins — identical binaries over identical inputs |
| vendored protos under `V3/Contracts/` | same four files committed in-repo |
| frozen Swagger 2.0 document, committed | `docs/openapi/quotes-v3.openapi.json`, byte-parity freeze |
| contract-drift CI workflow | `contract-drift` job diffing the frozen document |

## Pins

Verified August 2026 via the GitHub release API:

| Tool | Version | Released | Assets used |
|---|---|---|---|
| bufbuild/buf | **v1.50.0** | 2025-01-17 | `buf-Linux-x86_64`, `buf-Linux-aarch64` (exist on the tag) |
| grpc-gateway protoc-gen-openapiv2 | **v2.30.0** | 2026-08-05 | `protoc-gen-openapiv2-v2.30.0-linux-{x86_64,arm64}` |

Download URLs follow the .NET pattern verbatim:

- `https://github.com/bufbuild/buf/releases/download/v1.50.0/buf-Linux-$(uname -m)`
- `https://github.com/grpc-ecosystem/grpc-gateway/releases/download/v2.30.0/protoc-gen-openapiv2-v2.30.0-linux-${GOARCH}`
- checksums: `https://github.com/grpc-ecosystem/grpc-gateway/releases/download/v2.30.0/grpc-gateway_2.30.0_checksums.txt`

The `contracts` stage copies the .NET recipe: map `uname -m` → GOARCH, download both binaries,
then verify the plugin with `sha256sum | cut` compared against the `grep`ed line of the release
`checksums.txt` (buf is digest-verified only by the pinned URL; the plugin checksum matters because
that binary travels the network at build time). The stage also needs `protoc-gen-go`,
`protoc-gen-go-grpc` and `protoc-gen-grpc-gateway` (v2.30.0 binaries ship for the gateway plugin) —
installed via `go install <module>@<pin>` in the builder so the generated Go code and the gateway
runtime come from the versions [ADR 0002](0002-v3-transport-grpc-gateway.md) pins.

## Addendum: toolchain intermediate image (2026-08-26)

* Status: accepted · supersedes the inline download block, keeps every guarantee above.*

**Motivation** (review on the contracts PR): the hermetic `generate` stage re-downloaded and
re-sha256-verified both binaries on every `contract-drift` CI run and every local re-freeze —
repeated network work to reconstruct a state that never changes between pin bumps.

**Mechanism change**: download-time checksums became build-time checksums carried by digest
pinning. The exact download + verify recipe moved verbatim into
`contracts/docker-build-base/Dockerfile` (single-stage, no ENTRYPOINT, smoke-checks
`buf --version && protoc-gen-openapiv2 --version`, OCI label records the pin pair); the
`toolchain` workflow builds it multi-arch (`linux/amd64,linux/arm64`, gha-cached) and publishes
it as an immutable tag read from `contracts/docker-build-base/VERSION`:
`ghcr.io/josnelihurt/code.examples.go.quotes/toolchain/contracts:<VERSION>` — an immutable
version tag plus the digest pinned in `Dockerfile.build`, not a movable `latest`. `Dockerfile.build`
now `FROM`s that image digest-pinned; the verification ran once, at publish time, and the digest
carries it forward — every freeze runs exactly the bytes the workflow verified.

**Publish flow** (a pin bump is still a paired, re-freezing change across both repositories):

1. Edit `contracts/docker-build-base/VERSION` + the `ARG`s in its Dockerfile.
2. Publish — merge to `main` (or `workflow_dispatch`) runs the `toolchain` workflow; its summary
   prints the pushed digest.
3. Pin that digest on the `FROM` line of `Dockerfile.build` in the same PR.
4. Re-freeze via `scripts/update-contracts.sh` and commit the regenerated document.

**The .NET-repo pairing argument is unchanged**: identical binaries over identical inputs remains
the whole parity story — the binaries are the same two pins, only their provenance moved from
"downloaded and verified per build" to "published once and pinned by digest"; the local re-freeze
of the toolchain image reproduced the frozen document byte-for-byte. What weakens slightly is the
"offline-hermetic" wording above: a freeze now pulls one image from GHCR instead of two binaries
from GitHub releases — still fully pinned, and the drift job fails closed if the digest ever
disappears.
