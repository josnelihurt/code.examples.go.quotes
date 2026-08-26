package problemjson_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/problemjson"
)

// serveProblem runs the problem writer inside the correlation middleware so
// the envelope carries a correlation id exactly as it does in production.
func serveProblem(t *testing.T, write func(w http.ResponseWriter, r *http.Request), incomingCorrelation string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler := correlation.Middleware(http.HandlerFunc(write))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	if incomingCorrelation != "" {
		request.Header.Set(correlation.HeaderName, incomingCorrelation)
	}
	handler.ServeHTTP(recorder, request)

	return recorder
}

func TestWriteEmitsTheFullProblemBytes(t *testing.T) {
	recorder := serveProblem(t, func(w http.ResponseWriter, r *http.Request) {
		problemjson.Write(w, r, http.StatusTooManyRequests, "auth.rate_limited",
			"The auth endpoint rate limit was exceeded; retry after the window elapses.")
	}, "corr-42")

	assert.Equal(t, http.StatusTooManyRequests, recorder.Code)
	assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))

	body := recorder.Body.String()
	expected := `{"type":"https://tools.ietf.org/html/rfc9110#section-15.5.14",` +
		`"title":"Too Many Requests","status":429,` +
		`"detail":"The auth endpoint rate limit was exceeded; retry after the window elapses.",` +
		`"errorCode":"auth.rate_limited","correlationId":"corr-42"}` + "\n"
	assert.Equal(t, expected, body)
}

func TestWriteCarriesTheUnauthorizedEnvelope(t *testing.T) {
	recorder := serveProblem(t, func(w http.ResponseWriter, r *http.Request) {
		problemjson.Write(w, r, http.StatusUnauthorized, "auth.invalid_credentials", "Invalid credentials.")
	}, "")

	require.Equal(t, http.StatusUnauthorized, recorder.Code)

	body := recorder.Body.String()
	assert.Contains(t, body, `"type":"https://tools.ietf.org/html/rfc9110#section-15.5.2"`)
	assert.Contains(t, body, `"title":"Unauthorized"`)
	assert.Contains(t, body, `"status":401`)
	assert.Contains(t, body, `"detail":"Invalid credentials."`)
	assert.Contains(t, body, `"errorCode":"auth.invalid_credentials"`)
	// The middleware minted one and the envelope carried it.
	assert.Contains(t, body, `"correlationId":"`)
	// No valid span in the test context: traceId stays omitted, the way
	// ASP.NET Core leaves it absent for an invalid trace.
	assert.NotContains(t, body, `"traceId"`)
}

func TestWriteValidationEmitsTheErrorsMap(t *testing.T) {
	recorder := serveProblem(t, func(w http.ResponseWriter, r *http.Request) {
		problemjson.WriteValidation(w, r, http.StatusBadRequest, problemjson.RequestValidationErrorCode,
			map[string][]string{
				"Username": {"The Username field is required."},
			})
	}, "corr-7")

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))

	body := recorder.Body.String()
	expected := `{"type":"https://tools.ietf.org/html/rfc9110#section-15.5.1",` +
		`"title":"One or more validation errors occurred.","status":400,` +
		`"errors":{"Username":["The Username field is required."]},` +
		`"errorCode":"validation.request_invalid","correlationId":"corr-7"}` + "\n"
	assert.Equal(t, expected, body)
}

func TestTypeLinkMirrorsTheDotNetTable(t *testing.T) {
	cases := map[int]string{
		http.StatusBadRequest:          "https://tools.ietf.org/html/rfc9110#section-15.5.1",
		http.StatusUnauthorized:        "https://tools.ietf.org/html/rfc9110#section-15.5.2",
		http.StatusForbidden:           "https://tools.ietf.org/html/rfc9110#section-15.5.4",
		http.StatusNotFound:            "https://tools.ietf.org/html/rfc9110#section-15.5.5",
		http.StatusConflict:            "https://tools.ietf.org/html/rfc9110#section-15.5.10",
		http.StatusTooManyRequests:     "https://tools.ietf.org/html/rfc9110#section-15.5.14",
		http.StatusInternalServerError: "https://tools.ietf.org/html/rfc9110#section-15.6.1",
	}

	for status, link := range cases {
		assert.Equal(t, link, problemjson.TypeLink(status), "status %d", status)
	}
}

func TestTitlesComeFromTheReasonPhrases(t *testing.T) {
	recorder := serveProblem(t, func(w http.ResponseWriter, r *http.Request) {
		problemjson.Write(w, r, http.StatusInternalServerError, "error.unknown", "boom")
	}, "")

	assert.True(t, strings.Contains(recorder.Body.String(), `"title":"Internal Server Error"`))
}
