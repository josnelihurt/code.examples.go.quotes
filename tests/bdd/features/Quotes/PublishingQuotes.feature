Feature: Publishing quotes
  Maintainers add quotes to the catalog. The catalog rejects near-duplicates by
  fingerprint, so punctuation and casing cannot be used to smuggle the same quote in twice.

  Go port note: v3's create answers 200 with the created quote and no Location header
  (the transcoding runtime's shape), and rejections travel as the gRPC status envelope —
  the near-duplicate as code 6, the rule-breaking text as code 3.

  Background:
    Given the distributed application is running
    And I am signed in as "jrb"

  Scenario: A maintainer publishes a new quote
    When I publish a quote with unique text attributed to "Specification Suite"
    Then the response status is 200
    And the response body has "id"
    And the response carries no Location header
    And fetching the quote by its id returns the quote I published

  Scenario: A near-duplicate is rejected
    Given I have published a quote with unique text attributed to "Specification Suite"
    When I publish the same text with the final period replaced by an exclamation mark
    Then the response status is 409
    And the response is a grpc status envelope
    And the grpc status code is 6

  Scenario: Text that breaks the catalog rules is rejected
    When I publish a quote with the text "short"
    Then the response status is 400
    And the response is a grpc status envelope
    And the grpc status code is 3
