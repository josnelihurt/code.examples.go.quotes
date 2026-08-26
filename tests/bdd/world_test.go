package bdd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
)

// Base URLs and stack knobs, overridable through the environment so the same
// binary serves scripts/bdd.sh defaults, CI's compose project and a laptop
// whose ports are busy. The defaults are the host ports the BDD compose
// overlay publishes: the edge on 8080 (the .NET gateway parity URL), the
// quotes API on 8090 and the auth API on 8091 — the edge routes only the
// /api prefixes, so the documentation and health surfaces are addressed on
// the services themselves.
const (
	defaultBaseURL        = "http://localhost:8080"
	defaultQuotesAPIURL   = "http://localhost:8090"
	defaultAuthAPIURL     = "http://localhost:8091"
	defaultSigningKey     = "public-local-compose-signing-key-0000000000000000"
	defaultComposeProject = "quotes-bdd"
)

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func baseURL() string      { return envOrDefault("BDD_BASE_URL", defaultBaseURL) }
func quotesAPIURL() string { return envOrDefault("BDD_QUOTES_API_URL", defaultQuotesAPIURL) }
func authAPIURL() string   { return envOrDefault("BDD_AUTH_API_URL", defaultAuthAPIURL) }

// signingKey is the HS256 key the stack's authapi signs with (compose's
// AUTH_SIGNING_KEY default); token minting below must hold the same key or
// quotesapi would reject the result.
func signingKey() string { return envOrDefault("BDD_SIGNING_KEY", defaultSigningKey) }

// probe reports whether url answers any HTTP response within the budget —
// the reachability gate for the whole suite.
func probe(url string, timeout time.Duration) bool {
	client := &http.Client{Timeout: timeout}
	response, err := client.Get(url) //nolint:noctx // a reachability probe carries nothing
	if err != nil {
		return false
	}
	defer response.Body.Close() //nolint:errcheck // the probe only wants the status line
	return true
}

// stackReachable probes the three surfaces the suite speaks to.
func stackReachable() bool {
	return probe(baseURL()+"/", 2*time.Second) &&
		probe(quotesAPIURL()+"/health", 2*time.Second) &&
		probe(authAPIURL()+"/alive", 2*time.Second)
}

// devPasswords mirrors e2e/support/dev-users.ts in the frontend submodule:
// the specification vocabulary names users, and the throwaway development
// passwords resolve here (docs/dev-credentials.md is their one home).
var devPasswords = map[string]string{
	"jrb":    "supersecret",
	"reader": "readsecret",
}

// world is the per-scenario scratchpad: the bearer token the scenario runs
// with, the last response's status/headers/body, and the quote it published.
type world struct {
	client *http.Client

	// token is the bearer for catalog calls; empty means anonymous.
	token string
	// sentCorrelation is the X-Correlation-Id the last request carried, for
	// the echo assertion.
	sentCorrelation string

	status int
	header http.Header
	body   []byte

	publishedID     string
	publishedText   string
	publishedAuthor string
}

func newWorld() *world {
	return &world{client: &http.Client{Timeout: 15 * time.Second}}
}

// do sends request through the world's client, recording the response.
func (w *world) do(request *http.Request) error {
	if w.token != "" {
		request.Header.Set("Authorization", "Bearer "+w.token)
	}
	if request.Header.Get(correlationHeader) == "" {
		w.sentCorrelation = newCorrelationID()
		request.Header.Set(correlationHeader, w.sentCorrelation)
	} else {
		w.sentCorrelation = request.Header.Get(correlationHeader)
	}

	response, err := w.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close() //nolint:errcheck // the body is fully read below

	w.status = response.StatusCode
	w.header = response.Header
	if w.body, err = io.ReadAll(response.Body); err != nil {
		return fmt.Errorf("reading the response body: %w", err)
	}
	return nil
}

// doJSON sends request and decodes the response body as a JSON object. An
// empty body (the 403 the authorization middleware answers) is not an error:
// the caller reads the status and the empty map.
func (w *world) doJSON(request *http.Request) (map[string]any, error) {
	if err := w.do(request); err != nil {
		return nil, err
	}
	body := map[string]any{}
	if len(bytes.TrimSpace(w.body)) == 0 {
		return body, nil
	}
	if err := json.Unmarshal(w.body, &body); err != nil {
		return nil, fmt.Errorf("decoding the response body as JSON: %w", err)
	}
	return body, nil
}

// get issues GET url through the world.
func (w *world) get(url string) (map[string]any, error) {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return w.doJSON(request)
}

// getRaw issues GET url recording the response without touching the body —
// for surfaces that are not JSON (the Scalar reference page).
func (w *world) getRaw(url string) error {
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return w.do(request)
}

// postJSON issues POST url with the given object body.
func (w *world) postJSON(url string, payload any) (map[string]any, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(string(payloadBytes)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return w.doJSON(request)
}

// bodyJSON decodes the recorded body (for assertions on responses that were
// not JSON-driven requests, e.g. text/plain).
func (w *world) bodyJSON() (map[string]any, error) {
	body := map[string]any{}
	if err := json.Unmarshal(w.body, &body); err != nil {
		return nil, fmt.Errorf("decoding the response body as JSON: %w", err)
	}
	return body, nil
}

// mintToken issues an HS256 token as the auth context's own issuer would —
// the same claims builder authapi uses, signed with the stack's key, so a
// scenario can hold raw scopes without a login journey. Reusing
// infrastructure.NewJwtTokenService (rather than re-deriving the claim
// shapes here) is the point: issuer and audience drift is impossible.
func mintToken(username string, scopes []string) (string, error) {
	logger := slog.New(slog.DiscardHandler)
	service, err := infrastructure.NewJwtTokenService(
		&config.Jwt{SigningKey: signingKey()}, config.EnvironmentDevelopment, logger)
	if err != nil {
		return "", fmt.Errorf("building the minting service: %w", err)
	}
	issued, err := service.CreateToken(context.Background(), username, scopes)
	if err != nil {
		return "", fmt.Errorf("minting the token: %w", err)
	}
	return issued.AccessToken, nil
}

func newCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "correlation-unavailable"
	}
	return hex.EncodeToString(bytes)
}

// composeCommand returns "docker compose" or "podman compose" — the engine
// scripts/bdd.sh drove the stack with (BDD_COMPOSE_ENGINE when the script
// exported it, autodetected otherwise) — so the health scenario drives the
// same containers on the same engine every compose flavor supports.
func composeCommand() (string, string, error) {
	if engine := os.Getenv("BDD_COMPOSE_ENGINE"); engine != "" {
		return engine, "compose", nil
	}
	for _, engine := range []string{"docker", "podman"} {
		if _, err := exec.LookPath(engine); err == nil {
			return engine, "compose", nil
		}
	}
	return "", "", fmt.Errorf("no container runtime found: set BDD_COMPOSE_ENGINE to docker or podman")
}

// runCompose executes `compose <args...>` against the suite's compose
// project (default quotes-bdd — the project scripts/bdd.sh brought up) with
// the same file set the script used: podman-compose resolves containers
// through the files, not the project name alone.
func runCompose(args ...string) error {
	engine, subcommand, err := composeCommand()
	if err != nil {
		return err
	}
	root := repoRoot()
	project := envOrDefault("BDD_COMPOSE_PROJECT", defaultComposeProject)
	files := []string{
		"-f", filepath.Join(root, "docker-compose.yaml"),
		"-f", filepath.Join(root, "tests", "bdd", "compose.bdd.yaml"),
	}
	full := append([]string{subcommand, "-p", project}, files...)
	full = append(full, args...)
	command := exec.Command(engine, full...) //nolint:gosec // fixed argv, no shell
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", engine, strings.Join(full, " "), err, output)
	}
	return nil
}

// repoRoot is the module root, derived from this file's location — the test
// binary's working directory (tests/bdd) is not a safe anchor for the compose
// file set.
func repoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}
