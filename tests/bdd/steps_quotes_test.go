package bdd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cucumber/godog"
)

// correlationHeader is the shared correlation header name.
const correlationHeader = "X-Correlation-Id"

// registerQuoteSteps binds the catalog vocabulary. The transport parameter
// the .NET features carried is kept verbatim — this backend serves v3 only
// (ADR 0002), so anything else is a vocabulary error, not a silent detour.
func registerQuoteSteps(ctx *godog.ScenarioContext, w *world) {
	ctx.Step(`^I request a random quote from "([^"]*)"$`, w.stepRequestRandomQuote)
	ctx.Step(`^I list quotes from "([^"]*)"$`, w.stepListQuotes)
	ctx.Step(`^I list page (\d+) with size (\d+) from "([^"]*)"$`, w.stepListPage)
	ctx.Step(`^I request the quote with id "([^"]*)" from "([^"]*)"$`, w.stepRequestQuoteByID)
	ctx.Step(`^I request the quote I published from "([^"]*)"$`, w.stepRequestPublishedQuote)
	ctx.Step(`^I publish a quote with the text "([^"]*)" through the "([^"]*)" transport$`, w.stepPublishText)
	ctx.Step(`^I publish a quote with the text "([^"]*)"$`, w.stepPublishBareText)
	ctx.Step(`^I publish a quote with unique text attributed to "([^"]*)"(?: through the "([^"]*)" transport)?$`, w.stepPublishUnique)
	ctx.Step(`^I have published a quote with unique text attributed to "([^"]*)"$`, w.stepHavePublishedUnique)
	ctx.Step(`^I publish the same text with the final period replaced by an exclamation mark$`, w.stepPublishNearDuplicate)
	ctx.Step(`^fetching the quote by its id returns the quote I published$`, w.stepFetchPublishedByID)
}

// transportPath expands the version vocabulary into the /api prefix — the
// one place a non-v3 version can be caught with a readable message.
func transportPath(version string) (string, error) {
	if version != "v3" {
		return "", fmt.Errorf("this backend serves the v3 transport only (got %q)", version)
	}
	return "/api/v3/quotes", nil
}

func (w *world) stepRequestRandomQuote(version string) error {
	path, err := transportPath(version)
	if err != nil {
		return err
	}
	_, err = w.get(baseURL() + path + "/random")
	return err
}

func (w *world) stepListQuotes(version string) error {
	path, err := transportPath(version)
	if err != nil {
		return err
	}
	_, err = w.get(baseURL() + path)
	return err
}

func (w *world) stepListPage(page, size int, version string) error {
	path, err := transportPath(version)
	if err != nil {
		return err
	}
	query := url.Values{"page": {fmt.Sprint(page)}, "pageSize": {fmt.Sprint(size)}}
	_, err = w.get(baseURL() + path + "?" + query.Encode())
	return err
}

func (w *world) stepRequestQuoteByID(id, version string) error {
	path, err := transportPath(version)
	if err != nil {
		return err
	}
	_, err = w.get(baseURL() + path + "/" + url.PathEscape(id))
	return err
}

func (w *world) stepRequestPublishedQuote(version string) error {
	if w.publishedID == "" {
		return fmt.Errorf("no quote was published in this scenario yet")
	}
	return w.stepRequestQuoteByID(w.publishedID, version)
}

// publish posts the pair and records the published quote on success.
func (w *world) publish(text, author string) error {
	path, err := transportPath("v3")
	if err != nil {
		return err
	}
	body, err := w.postJSON(baseURL()+path, map[string]string{"text": text, "author": author})
	if err != nil {
		return err
	}
	if w.status == 200 {
		id, _ := body["id"].(string)
		if id == "" {
			return fmt.Errorf("the publish response carried no id: %s", string(w.body))
		}
		w.publishedID, w.publishedText, w.publishedAuthor = id, text, author
	}
	return nil
}

func (w *world) stepPublishText(text, version string) error {
	if _, err := transportPath(version); err != nil {
		return err
	}
	return w.publish(text, "Specification Suite")
}

// stepPublishBareText is the PublishingQuotes wording (no transport suffix —
// the .NET suite ran it against v1's rules); here it is the same v3 journey.
func (w *world) stepPublishBareText(text string) error {
	return w.publish(text, "Specification Suite")
}

func (w *world) stepPublishUnique(author, version string) error {
	if version != "" {
		if _, err := transportPath(version); err != nil {
			return err
		}
	}
	return w.publish(uniqueQuoteText(), author)
}

func (w *world) stepHavePublishedUnique(author string) error {
	if err := w.publish(uniqueQuoteText(), author); err != nil {
		return err
	}
	if w.status != 200 {
		return fmt.Errorf("publishing the fixture quote: expected 200, got %d (%s)", w.status, string(w.body))
	}
	return nil
}

func (w *world) stepPublishNearDuplicate() error {
	if w.publishedText == "" {
		return fmt.Errorf("no quote was published in this scenario yet")
	}
	index := strings.LastIndex(w.publishedText, ".")
	if index < 0 {
		return fmt.Errorf("the published text %q does not end with a period", w.publishedText)
	}
	return w.publish(w.publishedText[:index]+"!"+w.publishedText[index+1:], w.publishedAuthor)
}

func (w *world) stepFetchPublishedByID() error {
	if w.publishedID == "" {
		return fmt.Errorf("no quote was published in this scenario yet")
	}
	if err := w.stepRequestQuoteByID(w.publishedID, "v3"); err != nil {
		return err
	}
	return w.stepBodyIsPublishedQuote()
}

// uniqueQuoteText mints rule-abiding unique text: the random suffix keeps
// the fingerprint fresh while the sentence keeps the catalog rules (length,
// word count, terminator).
func uniqueQuoteText() string {
	return fmt.Sprintf("The specification suite publishes this unique observation %s.", newCorrelationID())
}
