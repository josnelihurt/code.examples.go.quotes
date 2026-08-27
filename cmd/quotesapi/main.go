// Command quotesapi serves the v3 quotes transport (issue #8): a grpc-go
// server behind the grpc-gateway runtime, fronted by the HTTP authentication
// middleware that owns the 401/403 wire shapes, plus the frozen OpenAPI
// document, the Scalar reference page and the platform health endpoints. It
// is the composition root — the only place config, telemetry, the auth
// validator, the catalog adapters and the v3 transport meet (ADR 0002).
//
// Layout: main.go owns OS signals and boot orchestration; wire.go owns
// catalog open and HTTP/gRPC composition; auth_adapter.go is where the two
// bounded contexts meet.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	authinfra "github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/httpserver"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/quotes/infrastructure"
)

// serviceName names the service in telemetry and spans.
const serviceName = "quotesapi"

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
