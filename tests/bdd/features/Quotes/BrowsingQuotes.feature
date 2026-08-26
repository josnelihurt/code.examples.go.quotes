Feature: Browsing quotes
  Readers pull quotes from the catalog: a random one, one by id, or a page of the stable
  ordering. The catalog ships seeded, so there is always something to read.

  Go port note: v3 serves errors as the gRPC status envelope, so the .NET suite's
  problem-document assertions below became envelope assertions (code 5 for the unknown
  id, code 3 for the rejected page); the default page size is the domain's 20, under
  which the eight seed rows are one page.

  Background:
    Given the distributed application is running
    And I am signed in as "jrb"

  Scenario: A random quote comes back with its text and author
    When I request a random quote from "v3"
    Then the response status is 200
    And the response body has "text" and "author"
    And the X-Correlation-Id header is echoed

  Scenario: A quote can be fetched again by its id
    Given I have published a quote with unique text attributed to "Specification Suite"
    When I request the quote I published from "v3"
    Then the response status is 200
    And the response body is the quote I published

  Scenario: An unknown id is a clean 404
    When I request the quote with id "00000000000000000000000000000000" from "v3"
    Then the response status is 404
    And the response is a grpc status envelope
    And the grpc status code is 5

  Scenario: Listing without parameters honors the default paging
    When I list quotes from "v3"
    Then the response status is 200
    And the response reports page 1 with the default page size

  Scenario: A page request outside the allowed range is rejected
    When I list page 0 with size 10 from "v3"
    Then the response status is 400
    And the response is a grpc status envelope
    And the grpc status code is 3
