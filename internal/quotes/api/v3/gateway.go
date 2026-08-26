package v3

import (
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
)

// NewGatewayMux builds the gateway ServeMux with every knob ADR 0002 pins.
// The generated handlers (RegisterQuoteServiceHandler*) mount onto it; the
// mux reads the google.api.http rules from the contract, so no hand-written
// routing exists anywhere in this transport.
//
// The knobs, knob by knob:
//
//   - Marshaler: JSONPb with MarshalOptions.EmitUnpopulated pinned explicitly
//     (the gateway default today; a future default flip must not drift the
//     error envelope silently) — it is the one knob that keeps "details":[]
//     in every error body — plus DiscardUnknown so unknown body members are
//     tolerated like every other transport's JSON binding.
//   - Incoming header matcher: X-Correlation-Id reaches the grpc layer as
//     x-correlation-id metadata (DefaultHeaderMatcher drops X- headers: they
//     are not IANA permanent).
//   - Outgoing header matcher: metadata the grpc layer sends travels without
//     the Grpc-Metadata- prefix, x-correlation-id spelled back as
//     X-Correlation-Id. The response header echo itself is owned by the
//     platform correlation middleware outside the gateway (.NET
//     UseCorrelationId parity) — the grpc service does not re-send it, so the
//     wire stays single-valued like the .NET stack's.
//   - ErrorHandler: DefaultHTTPErrorHandler, untouched — it marshals
//     status.Convert(err).Proto() through the mux marshaler, which is where
//     {"code","message","details"} and the HTTP status table come from.
func NewGatewayMux() *runtime.ServeMux {
	return runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.HTTPBodyMarshaler{
			Marshaler: &runtime.JSONPb{
				MarshalOptions:   protojson.MarshalOptions{EmitUnpopulated: true},
				UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
			},
		}),
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			if key == "X-Correlation-Id" {
				return "x-correlation-id", true // metadata keys are lowercase
			}
			return runtime.DefaultHeaderMatcher(key)
		}),
		runtime.WithOutgoingHeaderMatcher(func(key string) (string, bool) {
			if key == "x-correlation-id" {
				return "X-Correlation-Id", true
			}
			return key, true
		}),
	)
}

// Routes mounts the v3 transport on the host mux: the gateway behind the
// authentication middleware, plus the transport's documentation endpoints.
// Health and liveness are the host's business, not the transport's.
func Routes(host *http.ServeMux, gateway http.Handler, auth Authenticator) {
	host.Handle("/api/v3/", RequireScope(auth)(gateway))
	host.HandleFunc("GET /openapi/v3.json", serveOpenAPIDocument)
	host.HandleFunc("GET /scalar", serveScalarPage)
}
