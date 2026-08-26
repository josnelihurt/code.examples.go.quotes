package main_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/api"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/httpserver"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
)

// These tests live beside the composition root (not under internal/auth/api)
// on purpose: the full stack composes the infrastructure adapters, and the
// depguard layering rules keep the api package clean of exactly that.

const wireSigningKey = "wire-test-signing-key-that-is-long-enough-1234"

// newStack builds the serving chain exactly as main composes it, minus the
// OTel wrapper (no exporter in tests): routes -> request logger -> correlation.
func newStack(t *testing.T, permitLimit int) http.Handler {
	t.Helper()
	return newStackWithKey(t, permitLimit, wireSigningKey)
}

func newStackWithKey(t *testing.T, permitLimit int, signingKey string) http.Handler {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	credentials, err := infrastructure.NewHardcodedCredentialStore(config.EnvironmentDevelopment)
	require.NoError(t, err)
	tokens, err := infrastructure.NewJwtTokenService(
		&config.Jwt{SigningKey: signingKey}, config.EnvironmentDevelopment, logger)
	require.NoError(t, err)

	service := application.NewAuthService(credentials, tokens)
	limiter := infrastructure.NewRateLimiter(permitLimit, time.Minute)

	mux := http.NewServeMux()
	api.New(service, limiter, telemetry.NewMetrics(), logger).Register(mux)
	mux.HandleFunc("GET "+httpserver.HealthPath, httpserver.HandleHealth(nil))
	mux.HandleFunc("GET "+httpserver.AlivePath, httpserver.HandleAlive())

	var handler http.Handler = mux
	handler = httpserver.RequestLogger(logger, handler)
	handler = correlation.Middleware(handler)
	return handler
}

func post(t *testing.T, stack http.Handler, path, body string, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	recorder := httptest.NewRecorder()
	stack.ServeHTTP(recorder, request)
	response := recorder.Result()

	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	var decoded map[string]any
	if len(bytes.TrimSpace(raw)) > 0 {
		require.NoError(t, json.Unmarshal(raw, &decoded), "body: %s", raw)
	}
	return response, decoded
}

// jsonInt reads a JSON number as an int. encoding/json decodes every number
// into float64, and every number these assertions touch is a small exact
// integer — a status, a lifetime in seconds — so comparing as int says what is
// meant and avoids an epsilon comparison that would only obscure it.
func jsonInt(t *testing.T, value any) int {
	t.Helper()
	number, ok := value.(float64)
	require.True(t, ok, "expected a JSON number, got %T", value)
	return int(number)
}

func login(t *testing.T, stack http.Handler, username, password string, headers map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	return post(t, stack, api.LoginRoute, body, headers)
}

// tokenPayload decodes the unverified payload of a compact JWS.
func tokenPayload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)

	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)

	var claims map[string]any
	require.NoError(t, json.Unmarshal(decoded, &claims))
	return claims
}

func TestLoginReturnsTheTokenAndEchoesTheCorrelationId(t *testing.T) {
	stack := newStack(t, 10)

	response, body := login(t, stack, "jrb", "supersecret",
		map[string]string{correlation.HeaderName: "corr-full-1"})

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	assert.Equal(t, "corr-full-1", response.Header.Get(correlation.HeaderName))

	// The exact LoginResponseDto field set — no more, no less.
	require.Len(t, body, 4)
	accessToken, isString := body["accessToken"].(string)
	require.True(t, isString, "accessToken is a string")
	assert.NotEmpty(t, accessToken)
	assert.Equal(t, "corr-full-1", body["correlationId"])
	assert.Equal(t, 3600, jsonInt(t, body["expiresIn"]))
	assert.Equal(t, "jrb", body["username"])
}

func TestIssuedTokensCarryTheDocumentedScopeVocabulary(t *testing.T) {
	stack := newStack(t, 10)

	_, body := login(t, stack, "jrb", "supersecret", nil)
	claims := tokenPayload(t, body["accessToken"].(string))

	assert.Equal(t, "auth-api", claims["iss"])
	audience, ok := claims["aud"].([]any)
	require.True(t, ok)
	require.Len(t, audience, 1)
	assert.Equal(t, "aspire-quotes-poc", audience[0])
	assert.Equal(t, "jrb", claims["sub"])
	assert.Equal(t, "jrb", claims["name"])
	assert.Equal(t, "quotes:read quotes:write", claims["scope"])
}

func TestTheReaderLoginMintsOnlyTheReadScope(t *testing.T) {
	stack := newStack(t, 10)

	_, body := login(t, stack, "reader", "readsecret", nil)
	claims := tokenPayload(t, body["accessToken"].(string))

	assert.Equal(t, "quotes:read", claims["scope"])
}

func TestLoginWithAWrongPasswordReturnsThe401Problem(t *testing.T) {
	stack := newStack(t, 10)

	response, body := login(t, stack, "jrb", "wrong", nil)

	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	assert.Equal(t, "application/problem+json", response.Header.Get("Content-Type"))
	assert.Equal(t, "https://tools.ietf.org/html/rfc9110#section-15.5.2", body["type"])
	assert.Equal(t, "Unauthorized", body["title"])
	assert.Equal(t, 401, jsonInt(t, body["status"]))
	assert.Equal(t, "Invalid credentials.", body["detail"])
	assert.Equal(t, "auth.invalid_credentials", body["errorCode"])
	correlationID, isString := body["correlationId"].(string)
	require.True(t, isString)
	assert.NotEmpty(t, correlationID)
	assert.Equal(t, correlationID, response.Header.Get(correlation.HeaderName))
}

func TestLoginWithAnUnknownUserReturnsTheSameProblemShape(t *testing.T) {
	stack := newStack(t, 10)

	_, unknownUser := login(t, stack, "nobody", "supersecret", nil)
	_, wrongPassword := login(t, stack, "jrb", "wrong", nil)

	// The two failures are indistinguishable apart from the correlation id.
	unknownUser["correlationId"] = "same"
	wrongPassword["correlationId"] = "same"
	assert.Equal(t, unknownUser, wrongPassword)
}

func TestLoginWithEmptyFieldsReturnsThe400ValidationProblem(t *testing.T) {
	stack := newStack(t, 10)

	response, body := login(t, stack, "", "", nil)

	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "application/problem+json", response.Header.Get("Content-Type"))
	assert.Equal(t, "One or more validation errors occurred.", body["title"])
	assert.Equal(t, "validation.request_invalid", body["errorCode"])
	assert.NotEmpty(t, body["correlationId"])

	errorsByField, ok := body["errors"].(map[string]any)
	require.True(t, ok)
	usernameErrors, ok := errorsByField["Username"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, usernameErrors)
	passwordErrors, ok := errorsByField["Password"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, passwordErrors)
}

func TestLoginWithAMalformedBodyReturnsThe400ValidationProblem(t *testing.T) {
	stack := newStack(t, 10)

	response, body := post(t, stack, api.LoginRoute, "{ this is not json", nil)

	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "application/problem+json", response.Header.Get("Content-Type"))
	assert.Equal(t, "validation.request_invalid", body["errorCode"])
	assert.Equal(t, "The request body could not be read as JSON.", body["detail"])
	assert.NotEmpty(t, body["correlationId"])
}

func TestLoginWithNoBodyReturnsThe400ValidationProblem(t *testing.T) {
	stack := newStack(t, 10)

	response, body := post(t, stack, api.LoginRoute, "", nil)

	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "validation.request_invalid", body["errorCode"])
	assert.NotEmpty(t, body["correlationId"])
}

func TestRequestsBeyondThePermitLimitGetThe429Problem(t *testing.T) {
	stack := newStack(t, 2)

	first, _ := login(t, stack, "jrb", "supersecret", nil)
	require.Equal(t, http.StatusOK, first.StatusCode)

	second, _ := login(t, stack, "jrb", "wrong", nil)
	require.Equal(t, http.StatusUnauthorized, second.StatusCode)

	third, body := login(t, stack, "jrb", "supersecret", nil)
	require.Equal(t, http.StatusTooManyRequests, third.StatusCode)
	assert.Equal(t, "application/problem+json", third.Header.Get("Content-Type"))
	assert.Equal(t, "https://tools.ietf.org/html/rfc9110#section-15.5.14", body["type"])
	assert.Equal(t, "Too Many Requests", body["title"])
	assert.Equal(t, 429, jsonInt(t, body["status"]))
	assert.Equal(t, "The auth endpoint rate limit was exceeded; retry after the window elapses.", body["detail"])
	assert.Equal(t, "auth.rate_limited", body["errorCode"])
	assert.NotEmpty(t, body["correlationId"])
	assert.Empty(t, third.Header.Get("Retry-After"), "the .NET rejection sends no Retry-After")
}

func TestTheRateLimitWindowIsSharedByBothAuthRoutes(t *testing.T) {
	stack := newStack(t, 2)

	first, _ := login(t, stack, "jrb", "supersecret", nil)
	require.Equal(t, http.StatusOK, first.StatusCode)

	second, _ := login(t, stack, "reader", "readsecret", nil)
	require.Equal(t, http.StatusOK, second.StatusCode)

	response, body := post(t, stack, api.ValidateRoute, `{"accessToken":"anything"}`, nil)
	require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
	assert.Equal(t, "auth.rate_limited", body["errorCode"])
}

func TestValidateAnswersValidForAnIssuedToken(t *testing.T) {
	stack := newStack(t, 10)

	_, issued := login(t, stack, "jrb", "supersecret", nil)
	token := issued["accessToken"].(string)

	response, body := post(t, stack, api.ValidateRoute,
		fmt.Sprintf(`{"accessToken":%q}`, token), nil)

	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Len(t, body, 2)
	assert.Equal(t, true, body["valid"])
	assert.Equal(t, "jrb", body["username"])
}

func TestValidatePrefersTheBodyTokenOverTheHeader(t *testing.T) {
	stack := newStack(t, 10)

	_, issued := login(t, stack, "jrb", "supersecret", nil)
	token := issued["accessToken"].(string)

	response, body := post(t, stack, api.ValidateRoute,
		fmt.Sprintf(`{"accessToken":%q}`, token),
		map[string]string{"Authorization": "Bearer not-a-jwt"})

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, true, body["valid"])
	assert.Equal(t, "jrb", body["username"])
}

func TestValidateAcceptsTheTokenFromTheAuthorizationHeader(t *testing.T) {
	stack := newStack(t, 10)

	_, issued := login(t, stack, "jrb", "supersecret", nil)
	token := issued["accessToken"].(string)

	response, body := post(t, stack, api.ValidateRoute, "",
		map[string]string{"Authorization": "Bearer " + token})

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, true, body["valid"])
	assert.Equal(t, "jrb", body["username"])
}

func TestValidateAnswers200ValidFalseForGarbageAndForeignTokens(t *testing.T) {
	stack := newStack(t, 10)

	garbage, garbageBody := post(t, stack, api.ValidateRoute, `{"accessToken":"not-a-jwt"}`, nil)
	require.Equal(t, http.StatusOK, garbage.StatusCode)
	assert.Equal(t, false, garbageBody["valid"])
	assert.Nil(t, garbageBody["username"], "username is null for an invalid token")

	// A token signed by another key: same wire answer as garbage.
	foreignStack := newStackWithKey(t, 10, strings.Repeat("f", 32))
	_, foreign := login(t, foreignStack, "jrb", "supersecret", nil)
	foreignToken := foreign["accessToken"].(string)

	response, body := post(t, stack, api.ValidateRoute,
		fmt.Sprintf(`{"accessToken":%q}`, foreignToken), nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, false, body["valid"])
	assert.Nil(t, body["username"])
}

func TestValidateWithoutAnyTokenReturnsThe400Problem(t *testing.T) {
	stack := newStack(t, 10)

	response, body := post(t, stack, api.ValidateRoute, "", nil)

	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "application/problem+json", response.Header.Get("Content-Type"))
	assert.Equal(t, "One or more validation errors occurred.", body["title"])
	assert.Equal(t, "auth.token_missing", body["errorCode"])
	errorsByField, ok := body["errors"].(map[string]any)
	require.True(t, ok)
	messages, ok := errorsByField["auth.token_missing"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"An access token is required."}, messages)
	assert.NotEmpty(t, body["correlationId"])
}

func TestTheHealthAndAliveEndpointsAnswer(t *testing.T) {
	stack := newStack(t, 10)

	for _, path := range []string{httpserver.HealthPath, httpserver.AlivePath} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		stack.ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusOK, recorder.Code, path)
	}
}

func TestTheCorrelationIdIsMintedWhenTheClientSendsNone(t *testing.T) {
	stack := newStack(t, 10)

	response, _ := login(t, stack, "jrb", "supersecret", nil)

	minted := response.Header.Get(correlation.HeaderName)
	assert.Regexp(t, `^[0-9a-f]{32}$`, minted)
}
