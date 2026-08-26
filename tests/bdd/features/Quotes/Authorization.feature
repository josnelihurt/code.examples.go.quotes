Feature: Authorization
  The Quotes API admits callers by scope. Anonymous callers are challenged; readers may
  read but not publish; maintainers may do both.

  Go port note: v3 is the only transport this backend serves (ADR 0002), so the
  challenge/read/publish journeys below run against the v3 routes through the edge —
  and create answers 200 (the transcoding runtime has no 201/Location story), the one
  drift the Authorization vocabulary inherits from it.

  Background:
    Given the distributed application is running

  Scenario: An anonymous caller is challenged
    When I request a random quote from "v3"
    Then the response status is 401
    And the response carries a WWW-Authenticate header

  Scenario: A reader can read but not publish
    Given I am signed in as "reader"
    When I request a random quote from "v3"
    Then the response status is 200
    When I publish a quote with unique text attributed to "Reader Should Not Publish"
    Then the response status is 403

  Scenario: A maintainer can publish
    Given I am signed in as "jrb"
    When I publish a quote with unique text attributed to "Specification Suite"
    Then the response status is 200
