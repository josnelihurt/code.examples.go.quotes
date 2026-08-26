// Package httpserver carries the HTTP host kit (ADR 0006): the readiness and
// liveness endpoints, the slog request logger, and the graceful-shutdown serve
// loop — the pieces every API shares regardless of its bounded context.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
)

// HealthPath and AlivePath are the readiness/liveness routes; both stay
// unauthenticated and untraced in every environment (orchestrators wire
// probes to them and cannot depend on auth or on a collector being up).
const (
	HealthPath = "/health"
	AlivePath  = "/alive"
)

// HealthBudget is the hard wall clock every readiness check answers within —
// a select guard rather than a context deadline because a socket frozen
// mid-read can ignore cooperative cancellation, and readiness must answer
// regardless (the .NET Task.WhenAny guard).
const HealthBudget = 5 * time.Second

// HandleHealth is the readiness endpoint: 200 "Healthy" when the optional
// check answers within HealthBudget, 503 JSON otherwise. A nil check is the
// self-only readiness the .NET auth API registers (it owns no dependencies).
func HandleHealth(check func(ctx context.Context) error) http.HandlerFunc {
	return handleHealthWithBudget(check, HealthBudget)
}

func handleHealthWithBudget(check func(ctx context.Context) error, budget time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if check == nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Healthy"))
			return
		}

		done := make(chan error, 1)
		go func() { done <- check(r.Context()) }()

		select {
		case err := <-done:
			if err == nil {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Healthy"))
				return
			}
			writeUnhealthy(w, err)
		case <-time.After(budget):
			writeUnhealthy(w, fmt.Errorf("the readiness check exceeded the %s budget", budget))
		}
	}
}

func writeUnhealthy(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "Unhealthy",
		"error":  err.Error(),
	})
}

// HandleAlive is the liveness endpoint: dependency-free, always 200 — the
// process is up even when its dependencies are not.
func HandleAlive() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Healthy"))
	}
}

// statusRecorder captures the response status for the request log line.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// RequestLogger writes one Info line per request — method, path, status,
// duration and correlationId — the UseSerilogRequestLogging analogue; the
// correlation middleware must run outside it so the id lands on the line.
func RequestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		logger.With("correlationId", correlation.FromContext(r.Context())).
			InfoContext(r.Context(), "request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", recorder.status,
				"duration", time.Since(start).String())
	})
}

// Serve runs the handler until ctx is cancelled (or the listener fails), then
// drains in-flight requests within the shutdown budget — the graceful-shutdown
// parity of the .NET host's SIGTERM handling.
func Serve(ctx context.Context, logger *slog.Logger, addr string, handler http.Handler) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() { serverErr <- server.Serve(listener) }()
	logger.Info("listening", "addr", listener.Addr().String())

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down: %w", err)
		}
		return nil
	}
}
