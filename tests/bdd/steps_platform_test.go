package bdd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cucumber/godog"
)

// registerPlatformSteps binds the platform vocabulary: the documentation
// surfaces (addressed on the service itself — the edge routes only the
// /api prefixes) and the readiness journey that stops the real catalog
// database container and watches /health degrade.
func registerPlatformSteps(ctx *godog.ScenarioContext, w *world) {
	ctx.Step(`^I open "([^"]*)" on the "([^"]*)" service$`, w.stepOpenOnService)
	ctx.Step(`^the catalog database container is stopped$`, w.stepStopCatalogDatabase)
	ctx.Step(`^the quotes API health endpoint reports unhealthy$`, w.stepHealthReportsUnhealthy)
}

func (w *world) stepOpenOnService(path, service string) error {
	var target string
	switch service {
	case "quotes-api":
		target = quotesAPIURL()
	default:
		return fmt.Errorf("unknown service %q (this stack documents quotes-api only)", service)
	}
	return w.getRaw(target + path)
}

func (w *world) stepStopCatalogDatabase() error {
	return runCompose("stop", "postgres")
}

// stepHealthReportsUnhealthy polls the readiness endpoint until it answers
// 503 (the pool notices the stopped database within its ping budget), then
// restores the database and waits for readiness again — the compose stack is
// shared by every scenario of the run, so the journey must leave it healthy.
func (w *world) stepHealthReportsUnhealthy() error {
	if err := pollStatus(w.client, quotesAPIURL()+"/health", 503, 30*time.Second); err != nil {
		return err
	}

	if err := runCompose("start", "postgres"); err != nil {
		return fmt.Errorf("restoring the catalog database: %w", err)
	}
	if err := pollStatus(w.client, quotesAPIURL()+"/health", 200, 120*time.Second); err != nil {
		return fmt.Errorf("the catalog database did not come back: %w", err)
	}
	return nil
}

// pollStatus polls url until it answers the wanted status, failing after
// the budget. Connection errors keep polling until the budget is spent —
// a database coming back up refuses sockets for a while first.
func pollStatus(client *http.Client, url string, want int, budget time.Duration) error {
	deadline := time.Now().Add(budget)
	for {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			status := response.StatusCode
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			if status == want {
				return nil
			}
			if status != 200 && status != 503 {
				return fmt.Errorf("unexpected health status %d while polling for %d", status, want)
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no %d from %s within %s", want, url, budget)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
