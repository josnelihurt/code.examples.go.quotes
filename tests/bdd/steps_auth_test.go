package bdd

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// registerAuthSteps binds the credential vocabulary: real logins through the
// edge (the journey under test) and direct token minting for scenarios that
// need raw scopes without a login (the .NET StackSteps' token-issuing
// shorthand).
func registerAuthSteps(ctx *godog.ScenarioContext, w *world) {
	ctx.Step(`^I am signed in as "([^"]*)"$`, w.stepSignedInAs)
	ctx.Step(`^I sign in as "([^"]*)" with password "([^"]*)"$`, w.stepSignInWithPassword)
	ctx.Step(`^I introspect the current token$`, w.stepIntrospectCurrentToken)
	ctx.Step(`^I introspect the token "([^"]*)"$`, w.stepIntrospectToken)
	ctx.Step(`^I introspect without a token$`, w.stepIntrospectWithoutToken)
	ctx.Step(`^the introspection says the token is valid for "([^"]*)"$`, w.stepIntrospectionValidFor)
	ctx.Step(`^the introspection says the token is invalid$`, w.stepIntrospectionInvalid)
}

// login posts the credential pair at the edge's login route and records the
// response; the caller decides whether a non-200 is a failure (background
// sign-in) or an assertion target (the scenario's own journey).
func (w *world) login(username, password string) (map[string]any, error) {
	body, err := w.postJSON(baseURL()+"/api/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return nil, err
	}
	if w.status == 200 {
		if token, ok := body["accessToken"].(string); ok && token != "" {
			w.token = token
		}
	}
	return body, nil
}

func (w *world) stepSignedInAs(username string) error {
	password, known := devPasswords[username]
	if !known {
		return fmt.Errorf("unknown development user %q (docs/dev-credentials.md lists them)", username)
	}
	if _, err := w.login(username, password); err != nil {
		return err
	}
	if w.status != 200 {
		return fmt.Errorf("signing in as %q: expected 200, got %d (%s)", username, w.status, string(w.body))
	}
	return nil
}

func (w *world) stepSignInWithPassword(username, password string) error {
	_, err := w.login(username, password)
	return err
}

func (w *world) stepAuthenticatedDirectly(username, scopes string) error {
	token, err := mintToken(username, strings.Fields(scopes))
	if err != nil {
		return err
	}
	w.token = token
	return nil
}

func (w *world) stepIntrospectCurrentToken() error {
	if w.token == "" {
		return fmt.Errorf("no current token: sign in or mint one first")
	}
	_, err := w.postJSON(baseURL()+"/api/v1/auth/validate", map[string]string{"accessToken": w.token})
	return err
}

func (w *world) stepIntrospectToken(token string) error {
	_, err := w.postJSON(baseURL()+"/api/v1/auth/validate", map[string]string{"accessToken": token})
	return err
}

func (w *world) stepIntrospectWithoutToken() error {
	_, err := w.postJSON(baseURL()+"/api/v1/auth/validate", map[string]string{})
	return err
}

func (w *world) stepIntrospectionValidFor(username string) error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	if valid, _ := body["valid"].(bool); !valid {
		return fmt.Errorf("expected a valid token, got %s", string(w.body))
	}
	if got, _ := body["username"].(string); got != username {
		return fmt.Errorf("expected the token to be valid for %q, got %q", username, got)
	}
	return nil
}

func (w *world) stepIntrospectionInvalid() error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	if valid, _ := body["valid"].(bool); valid {
		return fmt.Errorf("expected an invalid token, got %s", string(w.body))
	}
	return nil
}
