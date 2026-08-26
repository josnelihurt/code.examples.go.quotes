Feature: API documentation
  The Quotes API publishes its OpenAPI document and a Scalar reference page. They are
  part of the contract surface: if they disappear, clients and tooling break. The edge
  only routes the /api prefixes, so these surfaces are addressed on the service itself —
  through the host port the BDD compose overlay maps for it.

  Go port note: this backend serves one transport (v3), so the .NET suite's
  per-transport document matrix collapses to the single frozen document the contract
  pipeline generates and the API serves verbatim; the auth API publishes no reference
  surfaces in this port, so its scenario is not mirrored.

  Background:
    Given the distributed application is running

  Scenario: The Quotes API publishes its reference surfaces
    When I open "/scalar" on the "quotes-api" service
    Then the response status is 200
    When I open "/openapi/v3.json" on the "quotes-api" service
    Then the response status is 200

  Scenario: The transcoded transport serves the OpenAPI document generated from its proto
    Transcoded routes are invisible to a runtime explorer, so no runtime document exists;
    the freeze pipeline generates one from the contract itself and the API serves it verbatim.
    When I open "/openapi/v3.json" on the "quotes-api" service
    Then the response status is 200
