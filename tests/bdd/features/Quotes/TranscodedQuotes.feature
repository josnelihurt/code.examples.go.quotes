Feature: Transcoded quotes transport
  v3 serves the same catalog through stock gRPC-JSON transcoding: the google.api.http
  rules in the proto drive the routing, no adapter exists, and the platform runtime
  writes the responses. That fidelity to the stock runtime is the point — so this suite
  pins both what v3 shares with the other transports (paths, success payloads, auth)
  and what deliberately drifts: error bodies are the gRPC status envelope instead of
  problem+json, create answers 200 without a Location header.

  Go port note: the scenarios reach the catalog through the Traefik edge of the compose
  stack (the YARP gateway's role), and the background mints its token directly with the
  stack's signing key — the harness-side proof that a non-authapi issuer holding the
  shared HS256 key validates at quotesapi.

  Background:
    Given the distributed application is running
    And I am authenticated directly as "bdd" with the scopes "quotes:read quotes:write"

  Scenario: Reads succeed with the same payload shape as the other transports
    When I request a random quote from "v3"
    Then the response status is 200
    And the response body has "text" and "author"
    And the X-Correlation-Id header is echoed

  Scenario: Listing honors the default paging with every field present
    When I list quotes from "v3"
    Then the response status is 200
    And the response reports page 1 with the default page size

  Scenario: An unknown id is a clean 404 in the gRPC status envelope
    When I request the quote with id "00000000000000000000000000000000" from "v3"
    Then the response status is 404
    And the response is a grpc status envelope
    And the grpc status code is 5

  Scenario: A domain rejection answers 400 in the gRPC status envelope
    When I publish a quote with the text "Too short." through the "v3" transport
    Then the response status is 400
    And the response is a grpc status envelope
    And the grpc status code is 3
    And the grpc message mentions "at least 12 characters"

  Scenario: Publishing succeeds with 200 and no Location header
    When I publish a quote with unique text attributed to "Specification Suite" through the "v3" transport
    Then the response status is 200
    And the response body has "id"
    And the response carries no Location header
    When I request the quote I published from "v3"
    Then the response status is 200
    And the response body is the quote I published

  Scenario: A page request outside the allowed range is rejected through the gRPC channel
    When I list page 0 with size 10 from "v3"
    Then the response status is 400
    And the response is a grpc status envelope
    And the grpc status code is 3
