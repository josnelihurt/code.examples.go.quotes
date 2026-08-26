// Package main's wire tests are the TranscodedWireTests port: the real
// composed HTTP mux — platform middleware, authentication middleware, the
// grpc-gateway runtime, the real grpc server with its interceptors, and the
// use cases over a hand-written stub repository — under httptest, with JWTs
// minted by the real issuer over a test key. They live in the composition
// root (not internal/quotes/api/v3) because the layering guard keeps bounded
// contexts from importing each other outside composition, which is exactly
// where the .NET suite's WebApplicationFactory composition lives too.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authdomain "github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	authinfra "github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/api/v3"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/application"
	quotesdomain "github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
)

// wireSigningKey mints and validates the suite's tokens (32+ bytes, like the
// platform minimum demands).
const wireSigningKey = "wire-test-signing-key-that-is-long-enough-1234567890"

// stubRepository is the hand-written catalog: the same eight quotes the .NET
// QuoteApiFactory seeds (same ids, same authors), duplicate detection by
// fingerprint like the unique index it stands in for.
type stubRepository struct {
	mu     sync.Mutex
	quotes []*quotesdomain.Quote
}

func newStubRepository(t *testing.T) *stubRepository {
	t.Helper()
	seed := [][2]string{
		{"Simplicity is the ultimate sophistication.", "Leonardo da Vinci"},
		{"Code is like humor. When you have to explain it, it's bad.", "Cory House"},
		{"First, solve the problem. Then, write the code.", "John Johnson"},
		{"Experience is the name everyone gives to their mistakes.", "Oscar Wilde"},
		{"The only way to go fast is to go well.", "Robert C. Martin"},
		{"Make it work, make it right, make it fast.", "Kent Beck"},
		{"Programs must be written for people to read.", "Harold Abelson"},
		{"Talk is cheap. Show me the code.", "Linus Torvalds"},
	}
	repo := &stubRepository{}
	for i, entry := range seed {
		quote, err := quotesdomain.ReconstituteQuote(
			fmt.Sprintf("%d", i+1), entry[0], entry[1], quotesdomain.ComputeFingerprint(entry[0]))
		require.NoError(t, err)
		repo.quotes = append(repo.quotes, quote)
	}
	return repo
}

func (s *stubRepository) GetRandom(_ context.Context) (*quotesdomain.Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quotes[0], nil // deterministic; the suite pins shape, not chance
}

func (s *stubRepository) GetByID(_ context.Context, id string) (*quotesdomain.Quote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, quote := range s.quotes {
		if quote.ID == id {
			return quote, nil
		}
	}
	return nil, nil
}

func (s *stubRepository) List(_ context.Context, skip, take int) (quotesdomain.QuotePage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if skip >= len(s.quotes) {
		return quotesdomain.QuotePage{Items: []*quotesdomain.Quote{}, Total: len(s.quotes)}, nil
	}
	end := skip + take
	if end > len(s.quotes) {
		end = len(s.quotes)
	}
	return quotesdomain.QuotePage{
		Items: append([]*quotesdomain.Quote{}, s.quotes[skip:end]...),
		Total: len(s.quotes),
	}, nil
}

func (s *stubRepository) Add(_ context.Context, quote *quotesdomain.Quote) (quotesdomain.QuoteAddOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.quotes {
		if existing.Fingerprint.Value == quote.Fingerprint.Value {
			return quotesdomain.QuoteDuplicateFingerprint, nil
		}
	}
	s.quotes = append(s.quotes, quote)
	return quotesdomain.QuoteAdded, nil
}

// wireHarness is the composed API under test.
type wireHarness struct {
	server  *httptest.Server
	issuer  *authinfra.JwtTokenService
	repo    *stubRepository
	cleanup func()
}

// newWireHarness composes the same handler newHandler builds for run().
func newWireHarness(t *testing.T) *wireHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	repo := newStubRepository(t)
	issuer, err := authinfra.NewJwtTokenService(
		&config.Jwt{SigningKey: wireSigningKey}, config.EnvironmentDevelopment,
		slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	require.NoError(t, err)

	handler, shutdown, err := newHandler(ctx, slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), transportDeps{
		random:  application.NewGetRandomQuoteUseCase(repo),
		byID:    application.NewGetQuoteByIDUseCase(repo),
		list:    application.NewListQuotesUseCase(repo),
		create:  application.NewCreateQuoteUseCase(repo),
		metrics: telemetry.NewMetrics(),
		auth:    bearerAuthenticator{validator: issuer},
	})
	require.NoError(t, err)

	server := httptest.NewServer(handler)
	harness := &wireHarness{
		server: server,
		issuer: issuer,
		repo:   repo,
		cleanup: func() {
			server.Close()
			shutdown()
		},
	}
	t.Cleanup(harness.cleanup)
	return harness
}

// client returns an HTTP client pre-authenticated with a token carrying the
// scopes (the .NET CreateClient analogue); no scopes means no Authorization
// header at all.
func (h *wireHarness) client(t *testing.T, scopes ...string) *http.Client {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	if scopes == nil {
		return client
	}
	transport := &authorizedTransport{
		base:  http.DefaultTransport,
		token: h.token(t, scopes...),
	}
	client.Transport = transport
	return client
}

// token mints a real token from the real issuer with the granted scopes.
func (h *wireHarness) token(t *testing.T, scopes ...string) string {
	t.Helper()
	issued, err := h.issuer.CreateToken(context.Background(), "wire-suite", scopes)
	require.NoError(t, err)
	return issued.AccessToken
}

// authorizedTransport stamps the bearer token on every request.
type authorizedTransport struct {
	base  http.RoundTripper
	token string
}

func (a *authorizedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+a.token)
	if a.base == nil {
		return http.DefaultTransport.RoundTrip(clone)
	}
	return a.base.RoundTrip(clone)
}

func (h *wireHarness) get(t *testing.T, client *http.Client, target string) *http.Response {
	t.Helper()
	response, err := client.Get(h.server.URL + target)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

// postJSON posts the body with the client's own transport (auth included).
func (h *wireHarness) postJSON(t *testing.T, client *http.Client, body string) *http.Response {
	t.Helper()
	response, err := client.Post(h.server.URL+"/api/v3/quotes", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func readJSON(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	raw := new(bytes.Buffer)
	_, err := raw.ReadFrom(response.Body)
	require.NoError(t, err)
	require.NotEmpty(t, raw.String(), "the body must not be empty")
	parsed := map[string]any{}
	require.NoError(t, json.Unmarshal(raw.Bytes(), &parsed), raw.String())
	return parsed
}

// assertErrorEnvelope pins the gRPC status envelope every service failure
// answers with: {"code","message","details":[]} — deliberately different from
// the problem+json envelope the auth middleware owns.
func assertErrorEnvelope(t *testing.T, response *http.Response, status, code int, message string) {
	t.Helper()
	assert.Equal(t, status, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	body := readJSON(t, response)
	assert.Equal(t, float64(code), body["code"])
	assert.Equal(t, message, body["message"])
	assert.Equal(t, []any{}, body["details"])
}

// Fact 1 (and the success half of fact 12): the random quote answers the same
// camelCase body, and the correlation id is echoed on success.
func TestRandomAnswersTheSameCamelCaseQuoteBody(t *testing.T) {
	h := newWireHarness(t)

	request, err := http.NewRequest(http.MethodGet, h.server.URL+"/api/v3/quotes/random", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+h.token(t, authdomain.ScopeQuotesRead, authdomain.ScopeQuotesWrite))
	request.Header.Set("X-Correlation-Id", "wire-correlation-1")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	assert.Equal(t, "wire-correlation-1", response.Header.Get("X-Correlation-Id"))
	quote := readJSON(t, response)
	for _, member := range []string{"id", "text", "author"} {
		value, ok := quote[member].(string)
		require.True(t, ok, member)
		assert.NotEmpty(t, value, member)
	}
	assert.Len(t, quote, 3, "the quote carries exactly id, text and author")
}

// Fact 2: the list page shape with the paging scalars PRESENT on page one —
// the whole reason the contract declares them `optional`.
func TestListAnswersTheSamePageShapeWithPagingScalarsOnPageOne(t *testing.T) {
	h := newWireHarness(t)

	response := h.get(t, h.client(t, authdomain.ScopeQuotesRead, authdomain.ScopeQuotesWrite), "/api/v3/quotes?page=1&pageSize=3")

	require.Equal(t, http.StatusOK, response.StatusCode)
	page := readJSON(t, response)
	items, ok := page["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 3)
	for _, item := range items {
		entry, ok := item.(map[string]any)
		require.True(t, ok)
		assert.NotEmpty(t, entry["id"])
	}

	assert.Equal(t, float64(1), page["page"])
	assert.Equal(t, float64(3), page["pageSize"])
	assert.Equal(t, float64(8), page["totalItems"])
	assert.Equal(t, float64(3), page["totalPages"], "totalPages = ceil(totalItems / pageSize) with the seed's 8 items over 3")

	// The snake_case spellings bind too (ByTextName, then ByJSONName).
	snake := h.get(t, h.client(t, authdomain.ScopeQuotesRead), "/api/v3/quotes?page=1&page_size=2")
	require.Equal(t, http.StatusOK, snake.StatusCode)
	snakePage := readJSON(t, snake)
	assert.Equal(t, float64(2), snakePage["pageSize"])
}

// The .NET suite's get-by-id hit: quote 7 is Harold Abelson's.
func TestGetByIdAnswersTheSameQuote(t *testing.T) {
	h := newWireHarness(t)

	response := h.get(t, h.client(t, authdomain.ScopeQuotesRead), "/api/v3/quotes/7")

	require.Equal(t, http.StatusOK, response.StatusCode)
	quote := readJSON(t, response)
	assert.Equal(t, "7", quote["id"])
	assert.Equal(t, "Harold Abelson", quote["author"])
}

// Fact 3 (and the error half of fact 12): a miss answers the gRPC status
// envelope, not problem+json, with the correlation id still echoed.
func TestAMissingQuoteAnswersTheGrpcStatusEnvelopeNotProblemJson(t *testing.T) {
	h := newWireHarness(t)

	request, err := http.NewRequest(http.MethodGet, h.server.URL+"/api/v3/quotes/does-not-exist", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+h.token(t, authdomain.ScopeQuotesRead))
	request.Header.Set("X-Correlation-Id", "wire-correlation-2")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })

	assert.Equal(t, "wire-correlation-2", response.Header.Get("X-Correlation-Id"))
	assertErrorEnvelope(t, response, http.StatusNotFound, 5, "Quote not found.")
}

// Fact 4: create answers 200 with the quote and no Location header — the
// deliberate drift from every other transport.
func TestCreateAnswers200WithTheQuoteAndNoLocationHeader(t *testing.T) {
	h := newWireHarness(t)
	text := fmt.Sprintf("Transcoded creates answer 200 without Location %d.", time.Now().UnixNano())

	response := h.postJSON(t, h.client(t, authdomain.ScopeQuotesRead, authdomain.ScopeQuotesWrite),
		fmt.Sprintf(`{"text":%q,"author":"Transcoding Suite"}`, text))

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Empty(t, response.Header.Get("Location"))
	quote := readJSON(t, response)
	assert.NotEmpty(t, quote["id"])
	assert.Equal(t, text, quote["text"])
}

// Fact 5: a domain validation failure answers code 3 with the domain message.
func TestADomainValidationFailureAnswersCode3WithTheDomainMessage(t *testing.T) {
	h := newWireHarness(t)

	response := h.postJSON(t, h.client(t, authdomain.ScopeQuotesWrite),
		`{"text":"Short.","author":"Ada Lovelace"}`)

	assertErrorEnvelope(t, response, http.StatusBadRequest, 3,
		fmt.Sprintf("Quote text must be at least %d characters.", quotesdomain.MinTextLength))
}

// The .NET suite's empty-fields fact: no contract-level guards exist, so an
// empty body flows to the domain rules and answers with their message.
func TestEmptyFieldsFlowToDomainValidationInsteadOfAContractLayer(t *testing.T) {
	h := newWireHarness(t)

	response := h.postJSON(t, h.client(t, authdomain.ScopeQuotesWrite), `{"text":"","author":""}`)

	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	body := readJSON(t, response)
	assert.Equal(t, float64(3), body["code"])
	assert.Contains(t, body["message"], "at least")
	assert.Equal(t, []any{}, body["details"])
}

// Fact 6: a malformed body answers the gateway's own JSON message — the one
// deliberate text drift from .NET's transcoding writer, pinned as the
// platform's own ("Request JSON payload is not correctly formatted." is
// grpc-dotnet's text; this is the encoding/json parse error the gateway
// carries verbatim).
func TestAMalformedBodyAnswersTheGatewaysOwnJsonMessage(t *testing.T) {
	h := newWireHarness(t)

	response := h.postJSON(t, h.client(t, authdomain.ScopeQuotesWrite), "{ this is not json")

	assertErrorEnvelope(t, response, http.StatusBadRequest, 3,
		`invalid character 't' looking for beginning of object key string`)
}

// Fact 7: page=0 reaches the use case and answers the shared invalid-page
// message — presence, not truthiness, is what the contract's `optional` buys.
func TestAnInvalidPageRequestAnswersCode3WithTheSharedMessage(t *testing.T) {
	h := newWireHarness(t)

	response := h.get(t, h.client(t, authdomain.ScopeQuotesRead), "/api/v3/quotes?page=0")

	assertErrorEnvelope(t, response, http.StatusBadRequest, 3,
		"The requested page or page size is outside the allowed range.")
}

// Fact 8: an unauthenticated request answers the shared 401 problem — the one
// error path v3 never drifts on, byte-identical to the other transports'
// because the same shared problem factory builds it.
func TestAnUnauthenticatedRequestAnswersTheShared401Problem(t *testing.T) {
	h := newWireHarness(t)

	response := h.get(t, h.client(t), "/api/v3/quotes/random")

	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	assert.Equal(t, "application/problem+json", response.Header.Get("Content-Type"))
	assert.NotEmpty(t, response.Header.Get("WWW-Authenticate"))

	body := readJSON(t, response)
	assert.Equal(t, v3.TokenMissingErrorCode, body["errorCode"])
	assert.Equal(t, "A valid bearer token is required.", body["detail"])
	assert.Equal(t, "Unauthorized", body["title"])
	assert.Equal(t, "https://tools.ietf.org/html/rfc9110#section-15.5.2", body["type"])
	assert.Equal(t, float64(http.StatusUnauthorized), body["status"])
	assert.NotEmpty(t, body["correlationId"])
}

// Fact 9: a garbage token is the invalid-token 401, with the RFC 9110
// invalid_token challenge.
func TestAGarbageTokenAnswersTheInvalidToken401(t *testing.T) {
	h := newWireHarness(t)

	request, err := http.NewRequest(http.MethodGet, h.server.URL+"/api/v3/quotes/random", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer garbage-token")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })

	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	assert.Equal(t, `Bearer error="invalid_token"`, response.Header.Get("WWW-Authenticate"))
	body := readJSON(t, response)
	assert.Equal(t, "auth.token_invalid", body["errorCode"])
}

// Fact 10: a read-only token gets an empty 403, like every transport.
func TestAReadOnlyTokenGetsAnEmpty403(t *testing.T) {
	h := newWireHarness(t)

	response := h.postJSON(t, h.client(t, authdomain.ScopeQuotesRead),
		`{"text":"Transcoded writes need the write scope.","author":"Transcoding Suite"}`)

	require.Equal(t, http.StatusForbidden, response.StatusCode)
	raw := new(bytes.Buffer)
	_, err := raw.ReadFrom(response.Body)
	require.NoError(t, err)
	assert.Empty(t, raw.String())
}

// Fact 11: a duplicate fingerprint answers 409 with code 6 — the unique
// index's collision surfacing through the use case.
func TestADuplicateFingerprintCreateAnswers409Code6(t *testing.T) {
	h := newWireHarness(t)

	response := h.postJSON(t, h.client(t, authdomain.ScopeQuotesWrite),
		`{"text":"Talk is cheap. Show me the code!","author":"Someone Else Entirely"}`)

	assertErrorEnvelope(t, response, http.StatusConflict, 6,
		"A quote with the same meaning already exists.")
}

// Fact 13: the OpenAPI document is served from the frozen, generated
// artifact — what Scalar shows is what the contract-drift job diffs.
func TestTheV3OpenAPIDocumentIsServedFromTheProtoGeneratedArtifact(t *testing.T) {
	h := newWireHarness(t)

	response := h.get(t, h.client(t, authdomain.ScopeQuotesRead), "/openapi/v3.json")

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))

	var document struct {
		Swagger string `json:"swagger"`
		Info    struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths map[string]struct {
			Get struct {
				Summary string `json:"summary"`
			} `json:"get"`
		} `json:"paths"`
		SecurityDefinitions map[string]any   `json:"securityDefinitions"`
		Security            []map[string]any `json:"security"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&document))

	assert.Equal(t, "2.0", document.Swagger)
	assert.Equal(t, "Quotes.Api | v3", document.Info.Title)
	for _, path := range []string{"/api/v3/quotes", "/api/v3/quotes/random", "/api/v3/quotes/{id}"} {
		_, ok := document.Paths[path]
		assert.True(t, ok, "generated paths cover "+path)
	}

	// The summaries come from the proto's own leading comments.
	assert.Contains(t, document.Paths["/api/v3/quotes"].Get.Summary, "Lists one page of the quote catalog")

	// The bearer requirement is declared in the contract's openapiv2 options.
	assert.Contains(t, document.SecurityDefinitions, "Bearer")
	assert.NotEmpty(t, document.Security)
}

// Fact 14: the Scalar reference page serves, pointing at the frozen document.
func TestTheScalarPageServes(t *testing.T) {
	h := newWireHarness(t)

	response := h.get(t, h.client(t), "/scalar")

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "text/html; charset=utf-8", response.Header.Get("Content-Type"))
	page := new(bytes.Buffer)
	_, err := page.ReadFrom(response.Body)
	require.NoError(t, err)
	assert.Contains(t, page.String(), "Scalar.createApiReference")
	assert.Contains(t, page.String(), "/openapi/v3.json")
}

// The repeated-claims scope form (a JSON array) authorizes like the
// space-separated string the issuer mints — the middleware consults the
// normalized slice, so both forms take the same paths end to end.
func TestTheRepeatedClaimsScopeFormAuthorizesLikeTheStringForm(t *testing.T) {
	h := newWireHarness(t)

	now := time.Now()
	repeated, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":   "auth-api",
		"aud":   []string{"aspire-quotes-poc"},
		"sub":   "wire-suite",
		"name":  "wire-suite",
		"exp":   now.Add(time.Hour).Unix(),
		"scope": []any{authdomain.ScopeQuotesRead},
	}).SignedString([]byte(wireSigningKey))
	require.NoError(t, err)

	read := &authorizedTransport{token: repeated}
	response := h.get(t, &http.Client{Timeout: 10 * time.Second, Transport: read}, "/api/v3/quotes/random")
	require.Equal(t, http.StatusOK, response.StatusCode)

	denied := h.postJSON(t, &http.Client{Timeout: 10 * time.Second, Transport: read},
		`{"text":"Repeated claims cannot write.","author":"Transcoding Suite"}`)
	require.Equal(t, http.StatusForbidden, denied.StatusCode)
}
