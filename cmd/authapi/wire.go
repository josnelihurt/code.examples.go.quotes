package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/api"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/application"
	"github.com/josnelihurt/code.examples.go.quotes/internal/auth/infrastructure"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/config"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/httpserver"
	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
)

// newHandler composes the auth HTTP surface — the same chain run() serves and
// the wire tests exercise: credential store → JWT → AuthService → rate
// limiter → mux (login/validate + health/alive) → outside-in middleware
// (server spans → correlation → request logging → routes).
func newHandler(
	logger *slog.Logger,
	environment string,
	jwt *config.Jwt,
	permitLimit int,
	window time.Duration,
	metrics *telemetry.Metrics,
) (http.Handler, error) {
	credentials, err := infrastructure.NewHardcodedCredentialStore(environment)
	if err != nil {
		return nil, err
	}
	tokens, err := infrastructure.NewJwtTokenService(jwt, environment, logger)
	if err != nil {
		return nil, fmt.Errorf("building the token service: %w", err)
	}

	service := application.NewAuthService(credentials, tokens)
	limiter := infrastructure.NewRateLimiter(permitLimit, window)

	mux := http.NewServeMux()
	api.New(service, limiter, metrics, logger).Register(mux)
	mux.HandleFunc("GET "+httpserver.HealthPath, httpserver.HandleHealth(nil))
	mux.HandleFunc("GET "+httpserver.AlivePath, httpserver.HandleAlive())

	var handler http.Handler = mux
	handler = httpserver.RequestLogger(logger, handler)
	handler = correlation.Middleware(handler)
	handler = telemetry.HTTPHandler(serviceName, handler)
	return handler, nil
}
