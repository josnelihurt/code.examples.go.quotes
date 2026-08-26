package bdd

import (
	"fmt"

	"github.com/cucumber/godog"
)

// registerStackSteps binds the stack-level vocabulary. "The distributed
// application is running" is the suite's precondition sentence, carried over
// from the .NET features verbatim: here it asserts the compose stack still
// answers at the three surfaces the journeys speak to (TestMain already
// gated on them once — a container dying mid-run should fail the scenario,
// not skip it).
func registerStackSteps(ctx *godog.ScenarioContext, w *world) {
	ctx.Step(`^the distributed application is running$`, func() error {
		if !stackReachable() {
			return fmt.Errorf("the stack is not reachable (edge %s, quotesapi %s, authapi %s)",
				baseURL(), quotesAPIURL(), authAPIURL())
		}
		return nil
	})

	ctx.Step(`^I am authenticated directly as "([^"]*)" with the scopes "([^"]*)"$`, w.stepAuthenticatedDirectly)
}
