# syntax=docker/dockerfile:1

# Contracts toolchain intermediate image: the pinned buf + protoc-gen-openapiv2 pair that
# Dockerfile.build's generate stage runs from. Publishing it (workflow:
# .github/workflows/toolchain.yml, tag from the sibling VERSION file) moves the download +
# sha256 verification out of every contract-drift CI run and every local re-freeze into a
# single publish event — Dockerfile.build pins the pushed digest, and the digest carries
# the verified state forward. Bumping the pins means editing VERSION + the ARGs below,
# publishing, then re-pinning the digest (docs/contributing.md, "Contracts toolchain
# image"). No ENTRYPOINT on purpose: the image is a FROM base, not a runnable thing.

# Same base tag the inline stage used. Tag-pinned; digest pinning can follow when the
# publish needs it.
FROM debian:bookworm-slim
WORKDIR /contracts

# Same pins the .NET Dockerfile.build uses: identical binaries over identical inputs is
# the whole byte-parity argument (docs/adr/0003-openapi-generation-pipeline.md), so the
# versions move only in paired PRs across both repositories, re-freezing in the same
# change. Both binaries are verified against their published checksums because they
# travel the network at build time — here at toolchain-publish time, once, instead of at
# every freeze.
ARG BUF_VERSION=v1.50.0
ARG OPENAPIV2_VERSION=v2.30.0
# The tag/label pair the workflow passes from the VERSION file (kept in one spelling so
# the published tag and the recorded version cannot drift apart).
ARG TOOLCHAIN_VERSION=1.50.0-2.30.0

# Native arch per host (the same case mapping the previous inline stage used): the
# workflow publishes the linux/amd64 + linux/arm64 manifest through its multi-arch
# build, while local podman machines on macOS build their own arch natively instead of
# qemu-emulated amd64 ones (emulated Go binaries crash). The generators are
# deterministic and the document carries no arch stamp, so both arches freeze identical
# bytes.
RUN case "$(uname -m)" in \
      x86_64) GOARCH=x86_64;; \
      aarch64) GOARCH=arm64;; \
      *) echo "unsupported arch $(uname -m)" >&2; exit 1;; \
    esac \
 && apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl \
 && rm -rf /var/lib/apt/lists/* \
 && curl -fsSL -o /usr/local/bin/buf \
      "https://github.com/bufbuild/buf/releases/download/${BUF_VERSION}/buf-Linux-$(uname -m)" \
 && chmod +x /usr/local/bin/buf \
 && test "$(sha256sum /usr/local/bin/buf | cut -d' ' -f1)" = \
        "$(curl -fsSL "https://github.com/bufbuild/buf/releases/download/${BUF_VERSION}/sha256.txt" \
          | grep "buf-Linux-$(uname -m)$" | cut -d' ' -f1)" \
 && curl -fsSL -o /usr/local/bin/protoc-gen-openapiv2 \
      "https://github.com/grpc-ecosystem/grpc-gateway/releases/download/v${OPENAPIV2_VERSION#v}/protoc-gen-openapiv2-v${OPENAPIV2_VERSION#v}-linux-${GOARCH}" \
 && chmod +x /usr/local/bin/protoc-gen-openapiv2 \
 && test "$(sha256sum /usr/local/bin/protoc-gen-openapiv2 | cut -d' ' -f1)" = \
        "$(curl -fsSL "https://github.com/grpc-ecosystem/grpc-gateway/releases/download/v${OPENAPIV2_VERSION#v}/grpc-gateway_${OPENAPIV2_VERSION#v}_checksums.txt" \
          | grep "protoc-gen-openapiv2-v${OPENAPIV2_VERSION#v}-linux-${GOARCH}" | cut -d' ' -f1)"

LABEL org.opencontainers.image.title="code.examples.go.quotes contracts toolchain" \
      org.opencontainers.image.description="Pinned buf + protoc-gen-openapiv2 pair for the hermetic OpenAPI freeze (docs/adr/0003)" \
      org.opencontainers.image.source="https://github.com/josnelihurt/code.examples.go.quotes" \
      org.opencontainers.image.version="${TOOLCHAIN_VERSION}"

# Sanity: the published image must actually run both generators. protoc-gen-openapiv2's
# --version prints to stdout and exits 0 (buf's likewise), so a plain RUN fails the
# build if either binary is broken — the versions it prints are the pins the LABEL
# records.
RUN buf --version && protoc-gen-openapiv2 --version
