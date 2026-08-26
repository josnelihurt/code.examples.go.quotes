// Command quotesapi serves the v3 quotes transport (issue #8): a grpc-go
// server behind the grpc-gateway runtime, fronted by the HTTP authentication
// middleware that owns the 401/403 wire shapes, plus the frozen OpenAPI
// document, the Scalar reference page and the platform health endpoints. It
// is the composition root — the only place config, telemetry, the auth
// validator, the catalog adapters and the v3 transport meet (ADR 0002).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/domain"
	authinfra "github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/httpserver"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/api/v3"
	contractv3 "github.com/josnelihurt/code.examples.go.quotes/internal/quotes/api/v3/contract"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/infrastructure"
)

// serviceName names the service in telemetry and spans.
const serviceName = "quotesapi"

// tokenValidator is the auth context's validator port as this composition
// sees it — the JwtTokenService satisfies it structurally.
type tokenValidator interface {
	ValidateToken(ctx context.Context, accessToken string) (domain.ValidateResult, error)
}

// bearerAuthenticator adapts the auth context's validator onto the v3
// transport's Authenticator port: the composition root is where the two
// bounded contexts meet (the layering guard enforces exactly that). A
// validation failure is not an error here — ok=false is the transport's
// invalid-token 401.
type bearerAuthenticator struct {
	validator tokenValidator
}

// Authenticate returns the token's granted scopes when it is valid.
func (a bearerAuthenticator) Authenticate(ctx context.Context, bearerToken string) ([]string, bool) {
	result, err := a.validator.ValidateToken(ctx, bearerToken)
	if err != nil || !result.Valid {
		return nil, false
	}
	return result.Scopes, true
}

// transportDeps is everything newHandler composes the HTTP surface from.
type transportDeps struct {
	random  *application.GetRandomQuoteUseCase
	byID    *application.GetQuoteByIDUseCase
	list    *application.ListQuotesUseCase
	create  *application.CreateQuoteUseCase
	metrics *telemetry.Metrics
	auth    v3.Authenticator
	// ready is the readiness check /health answers; nil means the self-only
	// readiness (this process up, dependencies unprobed).
	ready func(ctx context.Context) error
}

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).
			Error("quotesapi terminated unexpectedly", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("loading the configuration: %w", err)
	}

	// This API owns the catalog database; a missing connection string is a
	// boot failure, never a first-request surprise.
	databaseURL := cfg.ConnectionStrings.QuotesDb
	if databaseURL == "" {
		return errors.New("connectionstrings.quotesdb (ConnectionStrings:quotesdb) is required")
	}

	level := new(slog.LevelVar) // Info by default; per-area overrides land here
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).
		With("service.name", serviceName, "environment", cfg.Environment)

	otelShutdown, err := telemetry.Setup(ctx, logger, os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), serviceName)
	if err != nil {
		return fmt.Errorf("installing the telemetry pipeline: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			logger.Warn("the telemetry shutdown did not flush cleanly", "error", err)
		}
	}()

	// The catalog is migrated before serving — idempotent, and serialized
	// across replicas by the migrator's advisory lock (the .NET host's
	// Database.MigrateAsync stance). openCatalog retries connection failures
	// for a bounded budget: compose's depends_on:condition ordering is
	// unreliable on podman-compose (ADR 0001), so the boot itself tolerates a
	// database that is still becoming healthy.
	pool, err := openCatalog(ctx, logger, databaseURL)
	if err != nil {
		return fmt.Errorf("opening the catalog database: %w", err)
	}
	defer pool.Close()

	repository := infrastructure.NewPostgresQuoteRepository(pool)
	validator, err := authinfra.NewJwtTokenService(&cfg.Jwt, cfg.Environment, logger)
	if err != nil {
		return fmt.Errorf("building the token validator: %w", err)
	}

	deps := transportDeps{
		random:  application.NewGetRandomQuoteUseCase(repository),
		byID:    application.NewGetQuoteByIDUseCase(repository),
		list:    application.NewListQuotesUseCase(repository),
		create:  application.NewCreateQuoteUseCase(repository),
		metrics: telemetry.NewMetrics(),
		auth:    bearerAuthenticator{validator: validator},
		ready:   func(ctx context.Context) error { return infrastructure.Ping(ctx, pool, infrastructure.PingBudget) },
	}

	handler, shutdownTransport, err := newHandler(ctx, logger, deps)
	if err != nil {
		return fmt.Errorf("composing the http surface: %w", err)
	}
	defer shutdownTransport()

	logger.Info("starting the quotes api", "addr", cfg.Server.Address)
	return httpserver.Serve(ctx, logger, cfg.Server.Address, handler)
}

// openCatalog migrates the catalog and opens its pool, retrying with capped
// exponential backoff while the database is unreachable — the in-process
// expression of the .NET host's WaitFor(db) stance. The bound is deliberate:
// a database that never appears fails the boot loudly (after the retry
// budget) instead of hanging forever, and a genuinely broken connection
// string fails the same way, just later. A shutdown signal cancels the wait
// immediately.
func openCatalog(ctx context.Context, logger *slog.Logger, databaseURL string) (*pgxpool.Pool, error) {
	const (
		attempts   = 20
		minBackoff = 500 * time.Millisecond
		maxBackoff = 3 * time.Second
	)

	for attempt := 1; ; attempt++ {
		err := infrastructure.Migrate(ctx, databaseURL)
		if err == nil {
			var pool *pgxpool.Pool
			if pool, err = infrastructure.NewPool(ctx, databaseURL); err == nil {
				return pool, nil
			}
		}
		if attempt == attempts {
			return nil, err
		}

		backoff := min(minBackoff<<(attempt-1), maxBackoff)
		logger.Warn("the catalog database is not ready yet; retrying",
			"attempt", attempt, "of", attempts, "backoff", backoff, "error", err)

		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(backoff):
		}
	}
}

// newHandler composes the full HTTP surface — the same composition run()
// serves and the wire tests exercise:
//
//	grpc server (scope interceptor, v3 service) on a loopback listener
//	  -> gateway mux (ADR 0002 knobs) via RegisterQuoteServiceHandlerFromEndpoint
//	  -> stdlib mux: auth middleware -> gateway routes, /openapi/v3.json,
//	     /scalar, /health (readiness incl. the quotesdb round-trip), /alive
//	  -> platform middleware, outside-in: server spans (probes excluded) ->
//	     correlation (so the request logger and every problem body carry the
//	     id) -> request logging -> routes.
//
// The returned shutdown stops the grpc server gracefully; callers invoke it
// after the HTTP server has drained.
func newHandler(ctx context.Context, logger *slog.Logger, deps transportDeps) (http.Handler, func(), error) {
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(v3.ScopeUnaryInterceptor(deps.auth)))
	contractv3.RegisterQuoteServiceServer(grpcServer, v3.NewService(
		deps.random, deps.byID, deps.list, deps.create, deps.metrics))

	// Loopback listener: the grpc server is an implementation detail of the
	// transport — the wire surface is HTTP, so nothing listens off-host.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("listening for the grpc loopback: %w", err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- grpcServer.Serve(listener) }()
	logger.Info("grpc transport listening", "addr", listener.Addr().String())

	gateway := v3.NewGatewayMux()
	// Insecure transport credentials are the grpc-gateway in-process idiom:
	// the loopback listener this process owns is the only peer, and the wire
	// surface — where TLS belongs — is the HTTP side.
	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := contractv3.RegisterQuoteServiceHandlerFromEndpoint(ctx, gateway, listener.Addr().String(), dialOptions); err != nil {
		grpcServer.Stop()
		return nil, nil, fmt.Errorf("registering the gateway handlers: %w", err)
	}

	host := http.NewServeMux()
	v3.Routes(host, gateway, deps.auth)
	host.HandleFunc("GET "+httpserver.HealthPath, httpserver.HandleHealth(deps.ready))
	host.HandleFunc("GET "+httpserver.AlivePath, httpserver.HandleAlive())

	var handler http.Handler = host
	handler = httpserver.RequestLogger(logger, handler)
	handler = correlation.Middleware(handler)
	handler = telemetry.HTTPHandler(serviceName, handler)

	shutdown := func() {
		done := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case err := <-serveErr:
			if !errors.Is(err, grpc.ErrServerStopped) {
				logger.Warn("the grpc transport stopped with an error", "error", err)
			}
		case <-time.After(10 * time.Second):
			grpcServer.Stop() // drained enough; shed the rest
		}
	}
	return handler, shutdown, nil
}
