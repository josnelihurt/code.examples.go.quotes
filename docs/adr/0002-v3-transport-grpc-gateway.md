# ADR 0002 — v3 transport: grpc-go server + grpc-gateway v2 runtime

* Status: accepted · Date: 2026-08-25*

## Context

Scope: the v3 JSON transport of `quotes.v3.QuoteService` (`/api/v3/quotes…`), a port of the .NET gRPC-JSON-transcoding stack on `origin/main`.

## Decision

Serve v3 with a **grpc-go server** fronted by the **grpc-gateway v2 runtime** reading the same
`google.api.http` annotations from `quotes_v3.proto` — the proto stays the single contract of
record, no hand-written adapter. The module versions are verified and listed under
[Pins](#pins) below.

## Wire semantics, knob by knob

- **(a) Error envelope** — `DefaultHTTPErrorHandler` marshals `status.Convert(err).Proto()` (a
  `google.rpc.Status`) with the mux marshaler: `{"code":5,"message":"…","details":[]}`, `code`
  numeric — but only if the marshaler emits unpopulated lists. The gateway **default** JSONPb does
  (`EmitUnpopulated: true`); stock protojson would *omit* `"details"` — the one knob that must stay
  pinned. Parity is JSON-value level (protojson adds spaces); drift tests parse-and-compare.
- **(b) `optional` paging scalars** — verified empirically against protojson v1.36.12 with a proto3
  `optional` descriptor: a *set* optional is always emitted (`"page":1` on page one ✓, `"page":0`
  when the client sends 0, so it reaches the use case); an *unset* one is omitted (synthetic oneof),
  matching .NET presence. Empty repeated emits `[]` — only observable on an empty page, which the
  domain rejects anyway.
- **(c) HTTP status mapping** — `runtime.HTTPStatusFromCode` (read from v2.30.0 source): 3→400,
  5→404, 6→409, 7→403, 16→401, 13→500 — exactly the table the .NET wire tests pin.
- **(d) Query binding** — `normalizeFieldPath` resolves keys by `ByTextName` then `ByJSONName`, so
  `?page=1&page_size=20` and `?pageSize=3` both bind; populating an optional marks presence, so
  `req.HasPage()` tells absent from sent (defaults 1/20 stay in the use case).
- **(e) X-Correlation-Id** — `DefaultHeaderMatcher` *drops* X- headers (not IANA permanent) and the
  default outgoing matcher prefixes everything `Grpc-Metadata-`; override both (snippet below), echo
  the id via `grpc.SendHeader`.
- **(f) Auth** — in .NET the 401 (problem+json, `WWW-Authenticate`, byte-identical to v1) and the
  empty-body 403 come from the HTTP pipeline **before** gRPC. Reproduce with a `net/http` middleware
  wrapping the gateway mux (JWT + scope → 401 problem+json / 403 empty) **plus** a grpc-go unary
  interceptor as defense in depth — never let 401/403 surface through `DefaultHTTPErrorHandler`,
  which sets `WWW-Authenticate` to the *status message*.
- **(g) `/openapi/v3.json` + `/scalar`** — `go:embed docs/openapi/quotes-v3.openapi.json` served
  verbatim on the same mux, outside the gateway route table, plus a `/scalar` page pointing at it.

## Codegen (buf)

`buf.yaml` v2 + `buf.gen.yaml` v2 (mirroring the .NET `V3/Contracts` configs) with **local** plugins
from pinned binaries — not remote plugins, so generation stays hermetic and offline: `protoc-gen-go`,
`protoc-gen-go-grpc`, `protoc-gen-grpc-gateway`, `protoc-gen-openapiv2` ([ADR 0003](0003-openapi-generation-pipeline.md)). `go_package` =
`github.com/josnelihurt/code.examples.go.quotes/gen/quotesv3;quotesv3` via buf managed mode; wire
with `RegisterQuoteServiceHandlerFromEndpoint` over loopback/bufconn so the real `grpc.Server` and
its interceptors stay in the call path.

## Mux configuration (concrete)

```go
mux := runtime.NewServeMux(
    runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
        Marshaler: &runtime.JSONPb{ // pin explicitly; a gateway default flip must not drift silently
            MarshalOptions:   protojson.MarshalOptions{EmitUnpopulated: true}, // emits "details":[]
            UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
        },
    }),
    runtime.WithIncomingHeaderMatcher(func(k string) (string, bool) {
        if k == "X-Correlation-Id" { return "x-correlation-id", true } // metadata keys are lowercase
        return runtime.DefaultHeaderMatcher(k)
    }),
    runtime.WithOutgoingHeaderMatcher(func(k string) (string, bool) {
        if k == "x-correlation-id" { return "X-Correlation-Id", true } // echo; no Grpc-Metadata-
        return k, true
    }),
    // ErrorHandler: leave DefaultHTTPErrorHandler; matchers echo X-Correlation-Id both ways
)
```

The service maps failures exactly like `QuoteGrpcService.ToRpcException` — NotFound→5,
AlreadyExists/Conflict→6, Unauthenticated→16, PermissionDenied→7, Internal→13, else 3 — via
`status.Error(code, desc)`; create answers 200, no `Location`.

## Alternatives

**Connect RPC**: protocol-native errors carry *string* codes (`not_found`), not the numeric codes
v3's envelope pins (`"code":5`) — wire drift by construction. **Hand-written net/http adapter**:
that *is* v2 (`ProtoJsonResult` + adapter endpoints); v3's identity is "the stock platform runtime
serves the annotations".

## Consequences

+ Every v3 wire semantic the .NET drift tests pin maps to one explicit, verified gateway knob;
  numeric codes, status mapping and presence semantics come from the runtime, not our code.
− grpc server + gateway mux in one binary plus a buf codegen step; malformed-JSON bodies answer the
  gateway's own message text, not .NET's — pin the gateway's text in the port's tests.

## .NET mapping

| .NET v3 | Go port |
|---|---|
| `Microsoft.AspNetCore.Grpc.JsonTranscoding` | grpc-gateway v2 `ServeMux` + generated handlers |
| `RpcException(new Status(code, desc))` | `status.Error(code, desc)` |
| `request.HasPage ? Page : 1` | `req.HasPage()` / `req.GetPage()` on pointer scalars |
| `[Authorize]` + scope policies (HTTP pipeline) | mux middleware 401/403 + unary interceptor |

## Pins

Verified via the Go module proxy (v2.30.0 needs go ≥ 1.25):

| Module / tool | Version | Note |
|---|---|---|
| `google.golang.org/grpc` | **v1.83.2** | latest stable (v1.84/v1.85 are `-dev`; v1.74.0–1 retracted) |
| `google.golang.org/protobuf` (protoc-gen-go) | **v1.36.12** | protoc-gen-go ships inside this module |
| `google.golang.org/grpc/cmd/protoc-gen-go-grpc` | **v1.6.2** | |
| `github.com/grpc-ecosystem/grpc-gateway/v2` | **v2.30.0** (2026-08-05) | same tag the .NET repo pins for protoc-gen-openapiv2 — runtime and generator share one version. `genproto/googleapis/api` floats (no tags) — lock it in go.mod. |
