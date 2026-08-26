package correlation_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/baggage"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

// capture runs next behind the middleware and hands back the response
// recorder plus the correlation id the handler observed.
func capture(t *testing.T, headers map[string]string, next http.HandlerFunc) (*httptest.ResponseRecorder, string) {
	t.Helper()

	recorder := httptest.NewRecorder()
	observed := ""
	wrapped := correlation.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = correlation.FromContext(r.Context())
		if next != nil {
			next(w, r)
		}
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	wrapped.ServeHTTP(recorder, request)

	return recorder, observed
}

func TestTheMiddlewareEchoesAnIncomingCorrelationId(t *testing.T) {
	recorder, observed := capture(t, map[string]string{correlation.HeaderName: "incoming-123"}, nil)

	assert.Equal(t, "incoming-123", recorder.Header().Get(correlation.HeaderName))
	assert.Equal(t, "incoming-123", observed)
}

func TestTheMiddlewareMintsAGuidShapedIdWhenNoneIsSupplied(t *testing.T) {
	recorder, observed := capture(t, nil, nil)

	minted := recorder.Header().Get(correlation.HeaderName)
	require.NotEmpty(t, minted)
	assert.Regexp(t, hex32, minted)
	assert.Equal(t, minted, observed)
}

func TestTheMiddlewareIgnoresABlankHeader(t *testing.T) {
	recorder, _ := capture(t, map[string]string{correlation.HeaderName: "   "}, nil)

	assert.Regexp(t, hex32, recorder.Header().Get(correlation.HeaderName))
}

func TestTheMiddlewareAttachesTheBaggageMember(t *testing.T) {
	var observed baggage.Baggage
	recorder := httptest.NewRecorder()

	handler := correlation.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observed = baggage.FromContext(r.Context())
	}))
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	member := observed.Member("correlation.id")
	require.Equal(t, "correlation.id", member.Key(), "the correlation.id baggage member is attached")
	assert.Equal(t, recorder.Header().Get(correlation.HeaderName), member.Value())
}

func TestNewIDMintsUniqueHexIds(t *testing.T) {
	first, err := correlation.NewID()
	require.NoError(t, err)
	second, err := correlation.NewID()
	require.NoError(t, err)

	assert.Regexp(t, hex32, first)
	assert.Regexp(t, hex32, second)
	assert.NotEqual(t, first, second)
}

func TestFromContextIsBlankWithoutTheMiddleware(t *testing.T) {
	assert.Empty(t, correlation.FromContext(context.Background()))
}
