// Package telemetry installs the OpenTelemetry pipeline (ADR 0005): tracer
// and meter providers with OTLP gRPC exporters, enabled only when
// OTEL_EXPORTER_OTLP_ENDPOINT is set — no endpoint means the globals stay the
// SDK's no-op implementations and every instrumentation call is near-free. It
// also owns the custom counters (the .NET AppMetrics analogue), whose names
// and outcome values are public metric contract.
package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/httpserver"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// MeterName names the custom meter (the .NET AspireQuotesPoc counterpart).
const MeterName = "code.examples.go.quotes"

// Outcome vocabularies: the auth counters use the plain success/failure pair;
// the quotes counters use the ErrorOr outcome vocabulary ported verbatim.
const (
	OutcomeSuccess  = "success"
	OutcomeFailure  = "failure"
	OutcomeNotFound = "not_found"
	OutcomeError    = "error"
	OutcomeConflict = "conflict"
	OutcomeInvalid  = "invalid"
)

// Setup installs the tracer and meter providers backed by OTLP gRPC exporters
// when endpoint is non-empty, registering the W3C trace context + baggage
// propagators either way. The returned shutdown flushes and releases the
// providers; with an empty endpoint it is a no-op (graceful degradation —
// the globals remain no-ops).
func Setup(ctx context.Context, logger *slog.Logger, endpoint, serviceName string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if strings.TrimSpace(endpoint) == "" {
		logger.Info("otel exporter disabled: OTEL_EXPORTER_OTLP_ENDPOINT is not set")
		return func(context.Context) error { return nil }, nil
	}

	resource, err := sdkresource.Merge(sdkresource.Default(),
		sdkresource.NewWithAttributes(sdkresource.Default().SchemaURL(),
			attribute.String("service.name", serviceName)))
	if err != nil {
		return nil, err
	}

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint))
	if err != nil {
		return nil, err
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(resource),
	)

	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpoint(endpoint))
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, err
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(resource),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	logger.Info("otel exporters enabled", "endpoint", endpoint)

	return func(ctx context.Context) error {
		errTrace := tracerProvider.Shutdown(ctx)
		errMetric := meterProvider.Shutdown(ctx)
		if errTrace != nil {
			return errTrace
		}
		return errMetric
	}, nil
}

// HTTPHandler wraps a handler with server spans, skipping the health probes —
// the ASP.NET Core tracing filter parity. Uses the global providers, so it is
// a no-op pass-through when Setup installed nothing.
func HTTPHandler(serviceName string, next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, serviceName, otelhttp.WithFilter(func(r *http.Request) bool {
		return r.URL.Path != httpserver.HealthPath && r.URL.Path != httpserver.AlivePath
	}))
}

// Metrics holds the custom counters — one per use case, incremented with an
// outcome tag (the AppMetrics analogue). Created once at composition.
type Metrics struct {
	authLogin    metric.Int64Counter
	authValidate metric.Int64Counter

	quotesRandom  metric.Int64Counter
	quotesGetByID metric.Int64Counter
	quotesList    metric.Int64Counter
	quotesCreate  metric.Int64Counter
}

// NewMetrics builds the counters on the custom meter. Counter creation cannot
// fail with a plain name; a malformed name would panic here at boot, which is
// the fail-fast stance this kit takes.
func NewMetrics() *Metrics {
	meter := otel.Meter(MeterName)
	return &Metrics{
		authLogin:     mustCounter(meter, "auth.login.count", "Auth login attempts"),
		authValidate:  mustCounter(meter, "auth.validate.count", "Auth token validations"),
		quotesRandom:  mustCounter(meter, "quotes.random.count", "Random quote requests"),
		quotesGetByID: mustCounter(meter, "quotes.getbyid.count", "Get-quote-by-id requests"),
		quotesList:    mustCounter(meter, "quotes.list.count", "List quotes requests"),
		quotesCreate:  mustCounter(meter, "quotes.create.count", "Create quote requests"),
	}
}

func mustCounter(meter metric.Meter, name, description string) metric.Int64Counter {
	counter, err := meter.Int64Counter(name, metric.WithDescription(description))
	if err != nil {
		panic("telemetry: counter " + name + ": " + err.Error())
	}
	return counter
}

// RecordLogin increments the login counter with the auth outcome vocabulary
// (success|failure).
func (m *Metrics) RecordLogin(ctx context.Context, outcome string) {
	m.authLogin.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// RecordValidate increments the token-validation counter (success|failure).
func (m *Metrics) RecordValidate(ctx context.Context, outcome string) {
	m.authValidate.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// RecordQuotesRandom increments the random-quote counter with the ErrorOr
// outcome vocabulary (success|not_found|error|conflict|invalid).
func (m *Metrics) RecordQuotesRandom(ctx context.Context, outcome string) {
	m.quotesRandom.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// RecordQuotesGetByID increments the get-by-id counter.
func (m *Metrics) RecordQuotesGetByID(ctx context.Context, outcome string) {
	m.quotesGetByID.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// RecordQuotesList increments the list counter.
func (m *Metrics) RecordQuotesList(ctx context.Context, outcome string) {
	m.quotesList.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// RecordQuotesCreate increments the create counter.
func (m *Metrics) RecordQuotesCreate(ctx context.Context, outcome string) {
	m.quotesCreate.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}
