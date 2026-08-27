package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/httpserver"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/api/v3"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/api/v3/contract"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/infrastructure"
)

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
	contract.RegisterQuoteServiceServer(grpcServer, v3.NewService(
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
	if err := contract.RegisterQuoteServiceHandlerFromEndpoint(ctx, gateway, listener.Addr().String(), dialOptions); err != nil {
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
