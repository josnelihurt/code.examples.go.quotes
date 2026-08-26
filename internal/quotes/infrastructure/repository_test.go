package infrastructure

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/domain"
)

// defaultPostgresImage is the repo's single PostgreSQL pin (ADR 0001's
// docker.io/library/postgres:18.6-alpine3.23 — the same tag the compose stack
// and e2e boot; scripts/images.env lands with the orchestration layer and will
// become the resolver's source). POSTGRES_IMAGE overrides it for local
// experiments, mirroring the .NET PostgresTestDatabase fixture.
const defaultPostgresImage = "docker.io/library/postgres:18.6-alpine3.23"

func postgresImage() string {
	if override := os.Getenv("POSTGRES_IMAGE"); override != "" {
		return override
	}
	return defaultPostgresImage
}

// dbHarness is one PostgreSQL container per test run (ADR 0008) — started
// lazily, shared by every test in the package. Ryuk (testcontainers' reaper)
// reaps it where it can run; TestMain terminates it for the runtimes where
// Ryuk cannot (podman).
type dbHarness struct {
	container *postgres.PostgresContainer
	admin     *pgxpool.Pool
	adminDSN  string
}

var (
	harnessOnce sync.Once
	sharedH     *dbHarness
	harnessErr  error
)

func startHarness(ctx context.Context) (*dbHarness, error) {
	harnessOnce.Do(func() {
		container, err := postgres.Run(ctx, postgresImage(),
			postgres.WithDatabase("quotes"),
			postgres.WithUsername("quotes"),
			postgres.WithPassword("quotes"),
			postgres.BasicWaitStrategies(),
		)
		if err != nil {
			harnessErr = fmt.Errorf("starting the postgres container: %w", err)
			return
		}

		// The container's default database is the administration entry every
		// per-test database is created through (parity with PostgresTestDatabase).
		adminDSN, err := container.ConnectionString(ctx)
		if err != nil {
			harnessErr = fmt.Errorf("reading the container connection string: %w", err)
			return
		}
		admin, err := pgxpool.New(ctx, adminDSN)
		if err != nil {
			harnessErr = fmt.Errorf("opening the admin pool: %w", err)
			return
		}

		sharedH = &dbHarness{container: container, admin: admin, adminDSN: adminDSN}
	})

	return sharedH, harnessErr
}

// newMigratedDatabase provisions a fresh per-test database on the shared
// container, migrates it from zero through the embedded source, and returns
// the repository (and its pool) over it — every test starts from the exact
// catalog a production boot would hold.
func newMigratedDatabase(t *testing.T) (*PostgresQuoteRepository, *pgxpool.Pool) {
	t.Helper()
	skipWithoutContainerRuntime(t)

	ctx := context.Background()
	h, err := startHarness(ctx)
	if err != nil {
		t.Fatalf("starting the postgres harness: %v", err)
	}

	dsn, err := h.createTestDatabase(ctx)
	if err != nil {
		t.Fatalf("creating the per-test database: %v", err)
	}

	if err := Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrating the per-test database: %v", err)
	}

	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("opening the per-test pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return NewPostgresQuoteRepository(pool), pool
}

// createTestDatabase carves out quotes_test_<hex> on the shared container and
// returns its DSN.
func (h *dbHarness) createTestDatabase(ctx context.Context) (string, error) {
	name := "quotes_test_" + randomHex(8)
	if _, err := h.admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", name)); err != nil {
		return "", fmt.Errorf("creating %s: %w", name, err)
	}

	parsed, err := url.Parse(h.adminDSN)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + name

	return parsed.String(), nil
}

func randomHex(n int) string {
	buf := make([]byte, n)
	// crypto/rand.Read panics on entropy failure (Go 1.24+); a test id is not
	// a value an error would help the caller handle.
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// skipWithoutContainerRuntime keeps `go test ./...` green on machines with no
// container runtime: the suite runs only where DOCKER_HOST points somewhere or
// a docker/podman socket exists.
func skipWithoutContainerRuntime(t *testing.T) {
	t.Helper()

	if os.Getenv("DOCKER_HOST") != "" {
		return
	}

	candidates := []string{"/var/run/docker.sock"}
	if home, err := os.UserHomeDir(); err == nil {
		// The podman machine API socket (macOS and rootful Linux defaults).
		candidates = append(candidates,
			filepath.Join(home, ".local", "share", "containers", "podman", "machine", "default", "podman-api.sock"))
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		// The rootless podman socket.
		candidates = append(candidates, filepath.Join(xdg, "podman", "podman.sock"))
	}

	for _, socket := range candidates {
		if _, err := os.Stat(socket); err == nil {
			return
		}
	}

	t.Skipf("no container runtime detected (DOCKER_HOST unset and none of %v exist); "+
		"point DOCKER_HOST at a podman/docker socket to run the database suite", candidates)
}

// TestMain terminates the shared container after the run. Where Ryuk runs
// (docker, CI) this is a belt-and-braces no-op; where Ryuk is disabled (any
// podman endpoint, or TESTCONTAINERS_RYUK_DISABLED=true) the harness owns the
// container's life and this is the teardown.
func TestMain(m *testing.M) {
	disableRyukOnPodman()

	code := m.Run()
	terminateSharedHarness()

	os.Exit(code)
}

// terminateSharedHarness stops the run's container (kept in its own function
// so TestMain's os.Exit cannot strand a defer).
func terminateSharedHarness() {
	if sharedH == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sharedH.container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "terminating the postgres container: %v\n", err)
	}
}

// disableRyukOnPodman turns testcontainers' reaper off when the container
// endpoint is podman: the reaper container cannot run there (podman has no
// bridge network, and a podman-machine host socket cannot be bind-mounted
// into the VM). An explicit TESTCONTAINERS_RYUK_DISABLED always wins.
func disableRyukOnPodman() {
	if _, set := os.LookupEnv("TESTCONTAINERS_RYUK_DISABLED"); set {
		return
	}

	endpoint := dockerSocketPath()
	if endpoint == "" {
		return
	}
	if resolved, err := filepath.EvalSymlinks(endpoint); err == nil {
		endpoint = resolved
	}
	if strings.Contains(filepath.ToSlash(endpoint), "podman") {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

// dockerSocketPath resolves the container endpoint's socket file: DOCKER_HOST
// when it names a unix socket, otherwise the docker default.
func dockerSocketPath() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		parsed, err := url.Parse(host)
		if err != nil || parsed.Scheme != "unix" {
			return "" // a TCP endpoint is docker's world; Ryuk is fine there
		}
		return parsed.Path
	}

	return "/var/run/docker.sock"
}

// seededIDs is the shipped catalog: ids 1..8, sharing a fixed timestamp and
// tie-breaking lexically — the same rows the .NET migration seeds.
var seededIDs = [8]string{"1", "2", "3", "4", "5", "6", "7", "8"}

func TestGetRandomReturnsASeededQuote(t *testing.T) {
	repository, _ := newMigratedDatabase(t)

	quote, err := repository.GetRandom(context.Background())

	require.NoError(t, err)
	require.NotNil(t, quote)
	assert.Contains(t, seededIDs, quote.ID)
	// The stored fingerprint must be exactly what the Go algorithm derives
	// from the stored text — the seed values and ComputeFingerprint agree.
	assert.Equal(t, domain.ComputeFingerprint(quote.Text.Value), quote.Fingerprint.Value)
	assert.NotEmpty(t, quote.Author.Value)
}

func TestGetQuoteByIdFindsAndMisses(t *testing.T) {
	repository, _ := newMigratedDatabase(t)

	found, err := repository.GetByID(context.Background(), "3")

	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "First, solve the problem. Then, write the code.", found.Text.Value)
	assert.Equal(t, "John Johnson", found.Author.Value)
	assert.Equal(t, "first solve the problem then write the code", found.Fingerprint.Value)

	missing, err := repository.GetByID(context.Background(), "does-not-exist")

	require.ErrorIs(t, err, domain.NotFound())
	assert.Nil(t, missing)

	// A blank id never reaches the database and answers the same way.
	blank, err := repository.GetByID(context.Background(), "   ")

	require.ErrorIs(t, err, domain.NotFound())
	assert.Nil(t, blank)
}

func TestListPagesTheSeededCatalogInStableOrder(t *testing.T) {
	repository, _ := newMigratedDatabase(t)

	first, err := repository.List(context.Background(), 0, 3)
	require.NoError(t, err)
	second, err := repository.List(context.Background(), 3, 3)
	require.NoError(t, err)
	third, err := repository.List(context.Background(), 6, 3)
	require.NoError(t, err)

	require.Len(t, first.Items, 3)
	require.Len(t, second.Items, 3)
	require.Len(t, third.Items, 2)
	assert.Equal(t, 8, first.Total)
	assert.Equal(t, 8, second.Total)
	assert.Equal(t, 8, third.Total)

	// Seeds share created_at_utc, so the id tiebreaker orders the catalog
	// lexically: pages concatenate to 1..8 with no overlap.
	all := append(append(first.Items, second.Items...), third.Items...)
	ids := make([]string, 0, len(all))
	for _, quote := range all {
		ids = append(ids, quote.ID)
	}
	assert.Equal(t, []string{"1", "2", "3", "4", "5", "6", "7", "8"}, ids)

	// Repeated reads of the same page are stable.
	reread, err := repository.List(context.Background(), 0, 3)
	require.NoError(t, err)
	assert.Equal(t, ids[:3], quoteIDs(reread.Items))
}

func TestListReturnsAnEmptyPageBeyondTheEnd(t *testing.T) {
	repository, _ := newMigratedDatabase(t)

	page, err := repository.List(context.Background(), 80, 5)

	require.NoError(t, err)
	assert.Empty(t, page.Items)
	assert.Equal(t, 8, page.Total)
}

func TestListPlacesCreatedQuotesAfterTheSeeds(t *testing.T) {
	repository, _ := newMigratedDatabase(t)
	created, err := domain.NewQuote("Continuous delivery keeps software releasable.", "Jez Humble")
	require.NoError(t, err)
	outcome, err := repository.Add(context.Background(), created)
	require.NoError(t, err)
	require.Equal(t, domain.QuoteAdded, outcome)

	page, err := repository.List(context.Background(), 0, 100)

	require.NoError(t, err)
	require.Len(t, page.Items, 9)
	assert.Equal(t, 9, page.Total)
	// Seeds hold the fixed 2024-01-01 timestamp; a fresh creation sorts after
	// every one of them.
	assert.Equal(t, created.ID, page.Items[8].ID)
}

func TestAddRoundTripsThroughGetById(t *testing.T) {
	repository, _ := newMigratedDatabase(t)
	created, err := domain.NewQuote("Continuous delivery keeps software releasable.", "Jez Humble")
	require.NoError(t, err)

	outcome, err := repository.Add(context.Background(), created)

	require.NoError(t, err)
	assert.Equal(t, domain.QuoteAdded, outcome)

	loaded, err := repository.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, created.Text.Value, loaded.Text.Value)
	assert.Equal(t, created.Author.Value, loaded.Author.Value)
	assert.Equal(t, created.Fingerprint.Value, loaded.Fingerprint.Value)
}

func TestAddReportsADuplicateFingerprintAtomically(t *testing.T) {
	repository, _ := newMigratedDatabase(t)
	// The same text as seed 8 (a fresh instance with a fresh id, but the same
	// fingerprint): the unique index must reject it, not a check-then-insert.
	nearDuplicate, err := domain.NewQuote("Talk is cheap. Show me the code.", "Linus Torvalds")
	require.NoError(t, err)

	outcome, err := repository.Add(context.Background(), nearDuplicate)

	require.NoError(t, err)
	assert.Equal(t, domain.QuoteDuplicateFingerprint, outcome)

	page, err := repository.List(context.Background(), 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 8, page.Total)

	seed, err := repository.GetByID(context.Background(), "8")
	require.NoError(t, err)
	require.NotNil(t, seed)
	assert.Equal(t, "Talk is cheap. Show me the code.", seed.Text.Value)
}

func TestMigratingShipsTheSeededCatalogAndIsIdempotent(t *testing.T) {
	// A raw, unmigrated database — this test owns both Migrate calls so the
	// second one proves the no-change path is a success, not an error.
	skipWithoutContainerRuntime(t)
	ctx := context.Background()
	h, err := startHarness(ctx)
	if err != nil {
		t.Fatalf("starting the postgres harness: %v", err)
	}
	dsn, err := h.createTestDatabase(ctx)
	require.NoError(t, err)
	require.NoError(t, Migrate(ctx, dsn))
	require.NoError(t, Migrate(ctx, dsn))

	pool, err := NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	page, err := NewPostgresQuoteRepository(pool).List(ctx, 0, 100)

	require.NoError(t, err)
	require.Len(t, page.Items, 8)
	assert.Equal(t, seededIDs[:], quoteIDs(page.Items))
	assert.Equal(t, "Leonardo da Vinci", page.Items[0].Author.Value)
	assert.Equal(t, "Harold Abelson", page.Items[6].Author.Value)
}

func TestPingRoundTrips(t *testing.T) {
	_, pool := newMigratedDatabase(t)

	require.NoError(t, Ping(context.Background(), pool, PingBudget))
}

func quoteIDs(quotes []*domain.Quote) []string {
	ids := make([]string, 0, len(quotes))
	for _, quote := range quotes {
		ids = append(ids, quote.ID)
	}
	return ids
}
