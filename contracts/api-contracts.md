# API contracts

The contract of record for the v3 quotes transport is the proto file — everything
else is derived from it and regenerated, never hand-edited:

| What | Where |
|------|--------|
| Contract of record (v3) | [`contracts/quotes/v3/quotes_v3.proto`](../contracts/quotes/v3/quotes_v3.proto) |
| Frozen OpenAPI document (v3) | [`docs/openapi/quotes-v3.openapi.json`](../docs/openapi/quotes-v3.openapi.json) — generated, never hand-edited, drift-checked in CI |
| Generation pipeline | [`docs/adr/0003-openapi-generation-pipeline.md`](../docs/adr/0003-openapi-generation-pipeline.md) |

The document is produced by the hermetic `Dockerfile.build` `contracts` stage —
pinned buf + protoc-gen-openapiv2 over the proto and its vendored dependencies —
and the `contract-drift` CI job rebuilds it and fails on any mismatch with the
committed file.

After changing the contract:

```bash
./scripts/update-contracts.sh
```

Review the diff before committing: the proto is the input, the document is build
output.
