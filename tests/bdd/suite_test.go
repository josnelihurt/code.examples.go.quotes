// Package bdd is the specification suite (ADR 0008): the Reqnroll features
// ported to Gherkin verbatim-in-vocabulary, served by cucumber/godog running
// as a `go test` wrapper. The suite speaks HTTP through the Traefik edge of
// the compose stack the way the .NET suite spoke through the YARP gateway —
// no in-process host exists to boot here, so scripts/bdd.sh (locally and in
// the CI specs job) owns the stack's lifecycle and this package owns the
// journeys.
//
// Skipping: the suite is part of `go test ./...`, which CI's build-and-test
// job and Docker-less laptops run without any stack — TestMain probes the
// three base URLs first and exits 0 with a clear message when the stack is
// not reachable, so the suite only ever runs against something real.
package bdd

import (
	"fmt"
	"os"
	"testing"

	"github.com/cucumber/godog"
)

func TestMain(_ *testing.M) {
	// `go test ./...` must stay green with no stack running: probe before
	// handing control to godog and skip (exit 0, loudly) when it is down.
	if !stackReachable() {
		fmt.Printf("SKIP: the compose stack is not reachable at %s (quotesapi %s, authapi %s)\n",
			baseURL(), quotesAPIURL(), authAPIURL())
		fmt.Println("SKIP: run ./scripts/bdd.sh to bring the stack up and run the suite against it")
		os.Exit(0)
	}

	status := godog.TestSuite{
		Name:                 "bdd",
		TestSuiteInitializer: InitializeSuite,
		ScenarioInitializer:  InitializeScenario,
		Options: &godog.Options{
			Output:      os.Stdout,
			Paths:       []string{"features"},
			Format:      "pretty",
			Concurrency: 1, // scenarios share one seeded catalog: no interleaving
		},
	}.Run()
	os.Exit(status)
}

// InitializeSuite registers suite-level hooks (none today — the compose stack
// outlives the run and scripts/bdd.sh tears it down).
func InitializeSuite(*godog.TestSuiteContext) {}

// InitializeScenario binds the step vocabulary onto a fresh world per
// scenario — godog invokes it once per scenario, which is the isolation unit.
func InitializeScenario(ctx *godog.ScenarioContext) {
	w := newWorld()
	registerStackSteps(ctx, w)
	registerAuthSteps(ctx, w)
	registerQuoteSteps(ctx, w)
	registerResponseSteps(ctx, w)
	registerPlatformSteps(ctx, w)
}
