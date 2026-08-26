// Command authapi serves the auth API (issue #7): credential login and
// RFC 7662-style token introspection issuing HS256 JWTs. It is the
// composition root — the only place config, telemetry, the auth adapters and
// the transport meet.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/api"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/httpserver"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
)

// serviceName names the service in telemetry and spans.
const serviceName = "authapi"

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewJSONHandler(os.Stdout, nil)).
			Error("authapi terminated unexpectedly", "error", err)
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

	credentials, err := infrastructure.NewHardcodedCredentialStore(cfg.Environment)
	if err != nil {
		return err
	}
	tokens, err := infrastructure.NewJwtTokenService(&cfg.Jwt, cfg.Environment, logger)
	if err != nil {
		return err
	}

	service := application.NewAuthService(credentials, tokens)
	limiter := infrastructure.NewRateLimiter(
		cfg.RateLimiting.Auth.PermitLimit,
		time.Duration(cfg.RateLimiting.Auth.WindowSeconds)*time.Second)
	metrics := telemetry.NewMetrics()

	mux := http.NewServeMux()
	api.New(service, limiter, metrics, logger).Register(mux)
	mux.HandleFunc("GET "+httpserver.HealthPath, httpserver.HandleHealth(nil))
	mux.HandleFunc("GET "+httpserver.AlivePath, httpserver.HandleAlive())

	// Outside-in: server spans (health probes excluded) -> correlation (so the
	// request logger and every problem body carry the id) -> request logging ->
	// routes.
	var handler http.Handler = mux
	handler = httpserver.RequestLogger(logger, handler)
	handler = correlation.Middleware(handler)
	handler = telemetry.HTTPHandler(serviceName, handler)

	logger.Info("starting the auth api", "addr", cfg.Server.Address)
	return httpserver.Serve(ctx, logger, cfg.Server.Address, handler)
}
