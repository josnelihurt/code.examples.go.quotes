// Command authapi serves the auth API (issue #7): credential login and
// RFC 7662-style token introspection issuing HS256 JWTs. It is the
// composition root — the only place config, telemetry, the auth adapters and
// the transport meet.
//
// Layout: main.go owns OS signals and boot orchestration; wire.go owns
// newHandler (the mux + middleware onion the wire tests share).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
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

	handler, err := newHandler(
		logger,
		cfg.Environment,
		&cfg.Jwt,
		cfg.RateLimiting.Auth.PermitLimit,
		time.Duration(cfg.RateLimiting.Auth.WindowSeconds)*time.Second,
		telemetry.NewMetrics(),
	)
	if err != nil {
		return err
	}

	logger.Info("starting the auth api", "addr", cfg.Server.Address)
	return httpserver.Serve(ctx, logger, cfg.Server.Address, handler)
}
