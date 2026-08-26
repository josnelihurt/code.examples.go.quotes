// Package problemjson is the port's single RFC 9457 error envelope (ADR 0006)
// — the analogue of the .NET kit's ProblemDetailsBuilder/ProblemDetailsFactory
// pair. Every failure — transport validation, rejected credentials, rate
// limiting, the 401 challenge — renders through this helper so clients parse
// exactly one shape: application/problem+json carrying type/title/detail/
// status plus the errorCode, correlationId and traceId extensions.
package problemjson

import (
	"encoding/json"
	"net/http"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
	"go.opentelemetry.io/otel/trace"
)

// RequestValidationErrorCode is the errorCode carried by transport-level
// validation failures (body binding, field rules), whose errors map is keyed
// by property name rather than by error code.
const RequestValidationErrorCode = "validation.request_invalid"

// ValidationTitle mirrors the .NET kit's validation-problem title.
const ValidationTitle = "One or more validation errors occurred."

// contentType is the RFC 9457 media type every problem body carries.
const contentType = "application/problem+json"

// problem is the wire envelope. Field order mirrors the .NET serialization:
// the ProblemDetails members first, then the extensions. detail and errors are
// omitted when empty; extensions are omitted per-instance the same way the
// .NET factory omits a correlation id without an HttpContext.
type problem struct {
	Type          string              `json:"type"`
	Title         string              `json:"title"`
	Status        int                 `json:"status"`
	Detail        string              `json:"detail,omitempty"`
	Errors        map[string][]string `json:"errors,omitempty"`
	ErrorCode     string              `json:"errorCode"`
	CorrelationID string              `json:"correlationId,omitempty"`
	TraceID       string              `json:"traceId,omitempty"`
}

// Write emits the problem for a status, errorCode and detail. The type link
// and title derive from the status; the correlation id and trace id come from
// the request context (the span's TraceID, omitted when the trace is invalid —
// how ASP.NET Core stamps traceId).
func Write(w http.ResponseWriter, r *http.Request, status int, errorCode, detail string) {
	write(w, r, problem{
		Type:      TypeLink(status),
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		ErrorCode: errorCode,
	})
}

// WriteValidation emits the 400 validation problem: the shared envelope with
// an errors map keyed by field name (or, for service-level validation
// failures, by error code), mirroring HttpValidationProblemDetails.
func WriteValidation(w http.ResponseWriter, r *http.Request, status int, errorCode string, errorsByField map[string][]string) {
	write(w, r, problem{
		Type:      TypeLink(status),
		Title:     ValidationTitle,
		Status:    status,
		Errors:    errorsByField,
		ErrorCode: errorCode,
	})
}

func write(w http.ResponseWriter, r *http.Request, p problem) {
	if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
		p.TraceID = sc.TraceID().String()
	}
	if id := correlation.FromContext(r.Context()); id != "" {
		p.CorrelationID = id
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// TypeLink returns the RFC 9110 section link for a status — the table the .NET
// ProblemDetailsBuilder pins so every producer emits the same type.
func TypeLink(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "https://tools.ietf.org/html/rfc9110#section-15.5.2"
	case http.StatusForbidden:
		return "https://tools.ietf.org/html/rfc9110#section-15.5.4"
	case http.StatusNotFound:
		return "https://tools.ietf.org/html/rfc9110#section-15.5.5"
	case http.StatusConflict:
		return "https://tools.ietf.org/html/rfc9110#section-15.5.10"
	case http.StatusTooManyRequests:
		return "https://tools.ietf.org/html/rfc9110#section-15.5.14"
	case http.StatusInternalServerError:
		return "https://tools.ietf.org/html/rfc9110#section-15.6.1"
	default:
		return "https://tools.ietf.org/html/rfc9110#section-15.5.1"
	}
}
