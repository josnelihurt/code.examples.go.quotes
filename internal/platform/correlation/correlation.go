// Package correlation implements the correlation middleware (ADR 0006): every
// request carries an X-Correlation-Id — the client's when supplied, a minted
// 32-hex-character id otherwise (the Guid.ToString("N") shape) — echoed on the
// response, stored in the request context, stamped on the active span and the
// W3C baggage, and attached to the request log line.
package correlation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"
)

// HeaderName is the correlation header, shared with the .NET kit.
const HeaderName = "X-Correlation-Id"

// SpanAttributeKey and BaggageKey name the correlation member on spans and W3C
// baggage respectively.
const (
	SpanAttributeKey = "correlation.id"
	BaggageKey       = "correlation.id"
)

type contextKey struct{}

// NewID mints a correlation id: 16 crypto-random bytes, hex-encoded.
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Middleware resolves the request's correlation id (echoing the incoming
// header or minting one), echoes it on the response, and makes it visible
// downstream via the context, the active span and the baggage.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(HeaderName))
		if id == "" {
			// A mint failure means the entropy source is gone: the request still
			// serves, uncorrelated, rather than failing on telemetry grounds.
			if minted, err := NewID(); err == nil {
				id = minted
			}
		}

		if id != "" {
			w.Header().Set(HeaderName, id)
		}

		ctx := context.WithValue(r.Context(), contextKey{}, id)

		if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
			span.SetAttributes(attribute.String(SpanAttributeKey, id))
		}
		if member, err := baggage.NewMember(BaggageKey, id); err == nil {
			if merged, err := baggage.FromContext(ctx).SetMember(member); err == nil {
				ctx = baggage.ContextWithBaggage(ctx, merged)
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext returns the correlation id the middleware stashed — the .NET
// GetCorrelationId analogue (blank when no middleware ran, e.g. outside HTTP).
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}
