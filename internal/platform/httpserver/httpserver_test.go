package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josnelihurt/code.examples.go.quotes/internal/platform/correlation"
)

func TestHandleHealthWithoutACheckIsTheSelfProbe(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandleHealth(nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "text/plain; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "Healthy", recorder.Body.String())
}

func TestHandleHealthAnswers200WhenTheCheckPasses(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandleHealth(func(context.Context) error { return nil }).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "Healthy", recorder.Body.String())
}

func TestHandleHealthAnswers503JSONWhenTheCheckFails(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandleHealth(func(context.Context) error { return errors.New("the database is gone") }).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "Unhealthy", body["status"])
	assert.Contains(t, body["error"], "the database is gone")
}

func TestHandleHealthAnswers503WhenTheCheckExceedsTheBudget(t *testing.T) {
	recorder := httptest.NewRecorder()
	handleHealthWithBudget(func(ctx context.Context) error {
		<-ctx.Done() // a socket frozen mid-read: cooperative cancellation is ignored
		return nil
	}, 20*time.Millisecond).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "budget")
}

func TestHandleAliveIsAlwaysHealthy(t *testing.T) {
	recorder := httptest.NewRecorder()
	HandleAlive().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/alive", nil))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "Healthy", recorder.Body.String())
}

func TestRequestLoggerWritesOneLinePerRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	stack := correlation.Middleware(RequestLogger(logger, inner))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.Header.Set(correlation.HeaderName, "corr-log-1")
	stack.ServeHTTP(httptest.NewRecorder(), request)

	line := buf.String()
	require.Contains(t, line, `"msg":"request"`)
	assert.Contains(t, line, `"method":"POST"`)
	assert.Contains(t, line, `"path":"/api/v1/auth/login"`)
	assert.Contains(t, line, `"status":418`)
	assert.Contains(t, line, `"correlationId":"corr-log-1"`)
	assert.Contains(t, line, `"duration"`)
	assert.Equal(t, 1, strings.Count(line, "\n"), "exactly one line")
}

func TestServeShutsDownGracefullyWhenTheContextIsCancelled(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	errs := make(chan error, 1)
	go func() {
		errs <- Serve(ctx, logger, "localhost:0", handler)
	}()

	time.Sleep(50 * time.Millisecond) // let the listener bind
	cancel()

	select {
	case err := <-errs:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancellation")
	}
}

func TestServeSurfacesListenerErrors(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	blocked, err := net.Listen("tcp", "localhost:0") // occupy a port
	require.NoError(t, err)
	defer func() { _ = blocked.Close() }()

	err = Serve(context.Background(), logger, blocked.Addr().String(), http.NotFoundHandler())
	require.Error(t, err)
}
