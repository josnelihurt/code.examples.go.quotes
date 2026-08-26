package telemetry_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/telemetry"
)

// withManualMeter swaps the global meter provider for one backed by a manual
// reader, restoring the previous global on cleanup — the counters register on
// the global, so the swap must happen before NewMetrics.
func withManualMeter(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		_ = provider.Shutdown(context.Background())
	})

	return reader
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	return data
}

func findCounter(t *testing.T, data metricdata.ResourceMetrics, name string) metricdata.Sum[int64] {
	t.Helper()
	for _, scope := range data.ScopeMetrics {
		for _, metricValue := range scope.Metrics {
			if metricValue.Name != name {
				continue
			}
			sum, ok := metricValue.Data.(metricdata.Sum[int64])
			require.True(t, ok, "counter %s aggregates as a sum", name)
			return sum
		}
	}
	t.Fatalf("counter %s was not registered", name)
	return metricdata.Sum[int64]{}
}

func dataPointsByOutcome(sum metricdata.Sum[int64]) map[string]int64 {
	outcomes := make(map[string]int64)
	for _, point := range sum.DataPoints {
		for _, kv := range point.Attributes.ToSlice() {
			if kv.Key == attribute.Key("outcome") {
				outcomes[kv.Value.AsString()] = point.Value
			}
		}
	}
	return outcomes
}

func TestTheAuthCounterNamesAndOutcomeTagsArePinned(t *testing.T) {
	reader := withManualMeter(t)

	metrics := telemetry.NewMetrics()
	ctx := context.Background()

	metrics.RecordLogin(ctx, telemetry.OutcomeSuccess)
	metrics.RecordLogin(ctx, telemetry.OutcomeFailure)
	metrics.RecordValidate(ctx, telemetry.OutcomeFailure)

	data := collect(t, reader)

	login := findCounter(t, data, "auth.login.count")
	require.Len(t, login.DataPoints, 2)
	outcomes := dataPointsByOutcome(login)
	assert.Equal(t, int64(1), outcomes[telemetry.OutcomeSuccess])
	assert.Equal(t, int64(1), outcomes[telemetry.OutcomeFailure])

	validate := findCounter(t, data, "auth.validate.count")
	require.Len(t, validate.DataPoints, 1)
	assert.Equal(t, int64(1), dataPointsByOutcome(validate)[telemetry.OutcomeFailure])
}

func TestTheQuotesCounterNamesArePinned(t *testing.T) {
	reader := withManualMeter(t)

	metrics := telemetry.NewMetrics()
	ctx := context.Background()

	metrics.RecordQuotesRandom(ctx, telemetry.OutcomeSuccess)
	metrics.RecordQuotesGetByID(ctx, telemetry.OutcomeNotFound)
	metrics.RecordQuotesList(ctx, telemetry.OutcomeSuccess)
	metrics.RecordQuotesCreate(ctx, telemetry.OutcomeConflict)

	data := collect(t, reader)
	for _, name := range []string{
		"quotes.random.count", "quotes.getbyid.count", "quotes.list.count", "quotes.create.count",
	} {
		counter := findCounter(t, data, name)
		require.Len(t, counter.DataPoints, 1)
		assert.Equal(t, int64(1), counter.DataPoints[0].Value, name)
	}
}

func TestSetupWithoutAnEndpointIsANoOp(t *testing.T) {
	shutdown, err := telemetry.Setup(context.Background(),
		slog.New(slog.NewTextHandler(&discardBuffer{}, nil)), "", "authapi")
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
}

type discardBuffer struct{}

func (*discardBuffer) Write(p []byte) (int, error) { return len(p), nil }
