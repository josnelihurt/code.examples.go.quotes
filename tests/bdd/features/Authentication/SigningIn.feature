Feature: Signing in
  Local users exchange their credentials for an access token at the Auth API, reached
  through the edge. The token carries the scopes that decide what they may do at the
  Quotes API.

  Background:
    Given the distributed application is running

  Scenario: A maintainer signs in and reads a random quote through the edge
    When I sign in as "jrb" with password "supersecret"
    Then the response status is 200
    And the response body has "accessToken"
    When I request a random quote from "v3"
    Then the response status is 200
    And the response body has "text" and "author"

  Scenario: A wrong password is rejected
    When I sign in as "jrb" with password "not-the-password"
    Then the response status is 401
    And the problem errorCode is "auth.invalid_credentials"

  Scenario: Blank input is rejected as a validation problem
    When I sign in as "" with password ""
    Then the response status is 400
    And the response is a validation problem
