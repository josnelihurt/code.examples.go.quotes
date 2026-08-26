package v3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAuthenticator is the Authenticator port double for the unit tests: it
// answers from a fixed scope set. The end-to-end guards (real JWTs from the
// real issuer) live in the composition root's wire tests.
type fakeAuthenticator struct {
	scopes []string
	valid  bool
}

func (f fakeAuthenticator) Authenticate(_ context.Context, _ string) ([]string, bool) {
	return f.scopes, f.valid
}

// guardedHandler records that the request reached the gateway.
func guardedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestRequireScopeAnswersThe401ProblemForAMissingToken(t *testing.T) {
	handler := RequireScope(fakeAuthenticator{valid: true})(guardedHandler())

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v3/quotes/random", nil))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Equal(t, "Bearer", recorder.Header().Get("WWW-Authenticate"))
	assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))
	assert.Contains(t, recorder.Body.String(), `"errorCode":"`+TokenMissingErrorCode+`"`)
	assert.Contains(t, recorder.Body.String(), tokenRequiredDetail)
}

func TestRequireScopeAnswersThe401ProblemWithTheInvalidTokenCode(t *testing.T) {
	handler := RequireScope(fakeAuthenticator{valid: false})(guardedHandler())

	request := httptest.NewRequest(http.MethodGet, "/api/v3/quotes/random", nil)
	request.Header.Set("Authorization", "Bearer not-a-real-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Equal(t, `Bearer error="invalid_token"`, recorder.Header().Get("WWW-Authenticate"))
	assert.Contains(t, recorder.Body.String(), `"errorCode":"`+TokenInvalidErrorCode+`"`)
}

func TestRequireScopeAnswersAnEmpty403ForAnInsufficientScope(t *testing.T) {
	handler := RequireScope(fakeAuthenticator{valid: true, scopes: []string{ScopeRead}})(guardedHandler())

	request := httptest.NewRequest(http.MethodPost, "/api/v3/quotes", nil)
	request.Header.Set("Authorization", "Bearer a-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Empty(t, recorder.Body.String(), "the 403 body is empty, like the .NET middleware's")
	assert.Empty(t, recorder.Header().Get("Content-Type"))
}

func TestRequireScopePassesATokenWithTheRouteScope(t *testing.T) {
	handler := RequireScope(fakeAuthenticator{valid: true, scopes: []string{ScopeRead, ScopeWrite}})(guardedHandler())

	for _, target := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v3/quotes/random"},
		{http.MethodGet, "/api/v3/quotes"},
		{http.MethodGet, "/api/v3/quotes/7"},
		{http.MethodPost, "/api/v3/quotes"},
	} {
		request := httptest.NewRequest(target.method, target.path, nil)
		request.Header.Set("Authorization", "Bearer a-token")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusOK, recorder.Code, target.path)
	}
}

func TestRouteScopeMatchesTheAuthorizePolicies(t *testing.T) {
	cases := []struct {
		method, path string
		scope        string
		guarded      bool
	}{
		{http.MethodPost, "/api/v3/quotes", ScopeWrite, true},
		{http.MethodGet, "/api/v3/quotes", ScopeRead, true},
		{http.MethodGet, "/api/v3/quotes/random", ScopeRead, true},
		{http.MethodGet, "/api/v3/quotes/7", ScopeRead, true},
		{http.MethodPut, "/api/v3/quotes", "", false},    // no matching rpc policy
		{http.MethodGet, "/api/v3/other", "", false},     // not a v3 route
		{http.MethodPost, "/api/v3/quotes/7", "", false}, // create binds the collection only
	}
	for _, c := range cases {
		scope, guarded := routeScope(c.method, c.path)
		assert.Equal(t, c.guarded, guarded, c.method+" "+c.path)
		assert.Equal(t, c.scope, scope, c.method+" "+c.path)
	}
}

func TestParseBearerToleratesCaseAndWhitespace(t *testing.T) {
	token, ok := parseBearer("bearer some-token ")
	require.True(t, ok)
	assert.Equal(t, "some-token", token)

	_, ok = parseBearer("Basic dXNlcjpwYXNz")
	assert.False(t, ok)

	_, ok = parseBearer("Bearer ") // an empty candidate is no token at all
	assert.False(t, ok)

	_, ok = parseBearer("")
	assert.False(t, ok)
}
