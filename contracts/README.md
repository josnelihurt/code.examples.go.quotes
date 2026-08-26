# contracts

The v3 quotes transport's contract of record and its derivation pipeline. Everything
else is generated, never hand-edited — see [api-contracts.md](api-contracts.md), the
page the documentation set links to.

| What | Where |
|------|-------|
| Proto contract (v3) | [quotes/v3/quotes_v3.proto](quotes/v3/quotes_v3.proto) — messages, service and `google.api.http` rules, identical to the .NET original except for the `go_package` option swap and one load-bearing rpc reorder (`GetQuoteById` before `GetRandomQuote` — grpc-gateway pattern ties, see ADR 0002)'s |
| Vendored proto dependencies | `google/api/*` and `protoc-gen-openapiv2/options/*` beside it |
| buf generation configs | [quotes/v3/buf.gen.yaml](quotes/v3/buf.gen.yaml) (OpenAPI) and [quotes/v3/buf.gen.go.yaml](quotes/v3/buf.gen.go.yaml) (the committed Go contract code) |
| Frozen OpenAPI document | [docs/openapi/quotes-v3.openapi.json](../docs/openapi/quotes-v3.openapi.json) |

After changing the proto: `./scripts/update-contracts.sh` regenerates the frozen
document (hermetic `Dockerfile.build` stage, pinned buf + protoc-gen-openapiv2 —
[ADR 0003](../docs/adr/0003-openapi-generation-pipeline.md)); review the diff before
committing. The `contract-drift` CI job rebuilds the document and fails on any mismatch
with the committed bytes. Regenerating the Go code is `make contracts-go`.
