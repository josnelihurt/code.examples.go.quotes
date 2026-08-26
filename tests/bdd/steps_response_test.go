package bdd

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// registerResponseSteps binds the response vocabulary shared by every
// feature: status, payload members, correlation echo, and the two error
// shapes this platform serves — the RFC 9457 problem document (auth
// endpoints, v3's 401/403 pre-gateway answers) and the gRPC status envelope
// the transcoding runtime writes for everything the gateway surfaces.
func registerResponseSteps(ctx *godog.ScenarioContext, w *world) {
	ctx.Step(`^the response status is (\d+)$`, w.stepStatusIs)
	ctx.Step(`^the response body has "([^"]*)" and "([^"]*)"$`, w.stepBodyHas)
	ctx.Step(`^the response reports page 1 with the default page size$`, w.stepReportsDefaultPage)
	ctx.Step(`^the X-Correlation-Id header is echoed$`, w.stepCorrelationEchoed)
	ctx.Step(`^the response is a grpc status envelope$`, w.stepIsGrpcEnvelope)
	ctx.Step(`^the grpc status code is (\d+)$`, w.stepGrpcCodeIs)
	ctx.Step(`^the grpc message mentions "([^"]*)"$`, w.stepGrpcMessageMentions)
	ctx.Step(`^the problem errorCode is "([^"]*)"$`, w.stepProblemErrorCode)
	ctx.Step(`^the response is a validation problem$`, w.stepIsValidationProblem)
	ctx.Step(`^the response body has "([^"]*)"$`, w.stepBodyHasKey)
	ctx.Step(`^the response body is the quote I published$`, w.stepBodyIsPublishedQuote)
	ctx.Step(`^the response carries no Location header$`, w.stepCarriesNoLocation)
	ctx.Step(`^the response carries a Location header$`, w.stepCarriesLocation)
	ctx.Step(`^the response carries a WWW-Authenticate header$`, w.stepCarriesWWWAuthenticate)
}

func (w *world) stepStatusIs(status int) error {
	if w.status != status {
		return fmt.Errorf("expected status %d, got %d (%s)", status, w.status, string(w.body))
	}
	return nil
}

func (w *world) stepBodyHas(first, second string) error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	for _, key := range []string{first, second} {
		value, present := body[key]
		if !present || value == nil {
			return fmt.Errorf("the response body has no %q: %s", key, string(w.body))
		}
		if text, isString := value.(string); isString && text == "" {
			return fmt.Errorf("the response body's %q is empty", key)
		}
	}
	return nil
}

func (w *world) stepBodyHasKey(key string) error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	value, present := body[key]
	if !present || value == nil {
		return fmt.Errorf("the response body has no %q: %s", key, string(w.body))
	}
	if text, isString := value.(string); isString && text == "" {
		return fmt.Errorf("the response body's %q is empty", key)
	}
	return nil
}

// stepReportsDefaultPage pins the default paging: page 1 at the domain's
// default page size (20). Scenarios may have published more quotes earlier
// in the run (and a reused stack carries older rows), so only the values
// that cannot drift with catalog growth are pinned: page 1, pageSize 20,
// at most one page of items back, and at least the eight seed rows present.
func (w *world) stepReportsDefaultPage() error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	for key, expected := range map[string]float64{
		"page":     1,
		"pageSize": 20,
	} {
		got, _ := body[key].(float64)
		if got != expected {
			return fmt.Errorf("expected %s=%v, got %v (%s)", key, expected, got, string(w.body))
		}
	}
	totalItems, _ := body["totalItems"].(float64)
	if totalItems < 8 {
		return fmt.Errorf("expected at least the 8 seed rows, got %v totalItems", totalItems)
	}
	items, _ := body["items"].([]any)
	if len(items) > 20 {
		return fmt.Errorf("a default page must hold at most 20 items, got %d", len(items))
	}
	return nil
}

func (w *world) stepCorrelationEchoed() error {
	echoed := w.header.Get(correlationHeader)
	if echoed == "" {
		return fmt.Errorf("the response carried no %s header", correlationHeader)
	}
	if echoed != w.sentCorrelation {
		return fmt.Errorf("expected the sent %s %q to be echoed, got %q",
			correlationHeader, w.sentCorrelation, echoed)
	}
	return nil
}

func (w *world) stepIsGrpcEnvelope() error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	if _, isNumber := body["code"].(float64); !isNumber {
		return fmt.Errorf("the response is not a grpc status envelope (no numeric code): %s", string(w.body))
	}
	if _, isString := body["message"].(string); !isString {
		return fmt.Errorf("the response is not a grpc status envelope (no message): %s", string(w.body))
	}
	return nil
}

func (w *world) stepGrpcCodeIs(code int) error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	if got, _ := body["code"].(float64); int(got) != code {
		return fmt.Errorf("expected grpc status code %d, got %v (%s)", code, body["code"], string(w.body))
	}
	return nil
}

func (w *world) stepGrpcMessageMentions(fragment string) error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	message, _ := body["message"].(string)
	if !strings.Contains(message, fragment) {
		return fmt.Errorf("the grpc message %q does not mention %q", message, fragment)
	}
	return nil
}

func (w *world) stepProblemErrorCode(code string) error {
	if contentType := w.header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
		return fmt.Errorf("expected a problem document, got Content-Type %q (%s)", contentType, string(w.body))
	}
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	if got, _ := body["errorCode"].(string); got != code {
		return fmt.Errorf("expected problem errorCode %q, got %q (%s)", code, got, string(w.body))
	}
	return nil
}

func (w *world) stepIsValidationProblem() error {
	if contentType := w.header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
		return fmt.Errorf("expected a problem document, got Content-Type %q (%s)", contentType, string(w.body))
	}
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	errors, present := body["errors"].(map[string]any)
	if !present || len(errors) == 0 {
		return fmt.Errorf("expected a validation problem with field errors, got %s", string(w.body))
	}
	return nil
}

func (w *world) stepBodyIsPublishedQuote() error {
	body, err := w.bodyJSON()
	if err != nil {
		return err
	}
	for key, expected := range map[string]string{
		"id":     w.publishedID,
		"text":   w.publishedText,
		"author": w.publishedAuthor,
	} {
		if got, _ := body[key].(string); got != expected {
			return fmt.Errorf("the response's %q is %q, expected %q", key, got, expected)
		}
	}
	return nil
}

func (w *world) stepCarriesNoLocation() error {
	if location := w.header.Get("Location"); location != "" {
		return fmt.Errorf("expected no Location header, got %q", location)
	}
	return nil
}

func (w *world) stepCarriesLocation() error {
	if location := w.header.Get("Location"); location == "" {
		return fmt.Errorf("expected a Location header, got none (%s)", string(w.body))
	}
	return nil
}

func (w *world) stepCarriesWWWAuthenticate() error {
	if challenge := w.header.Get("WWW-Authenticate"); challenge == "" {
		return fmt.Errorf("expected a WWW-Authenticate header, got none (%s)", string(w.body))
	}
	return nil
}
