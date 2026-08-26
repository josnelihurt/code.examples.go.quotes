Feature: Token introspection
  Holders of a token can ask the Auth API whether it is still worth sending (RFC 7662
  style). Both answers are successes; only a request that carries no token at all is a
  client error.

  Background:
    Given the distributed application is running

  Scenario: A token issued this run introspects as valid for its user
    Given I am signed in as "jrb"
    When I introspect the current token
    Then the response status is 200
    And the introspection says the token is valid for "jrb"

  Scenario: A garbage token introspects as invalid
    When I introspect the token "not-a-real-token"
    Then the response status is 200
    And the introspection says the token is invalid

  Scenario: A request without any token is rejected
    When I introspect without a token
    Then the response status is 400
    And the problem errorCode is "auth.token_missing"
