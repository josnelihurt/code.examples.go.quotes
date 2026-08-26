Feature: Health readiness reflects the catalog database
  Orchestrators wire readiness to /health, so the endpoint must degrade when a real
  dependency goes away — a probe that cannot fail is worse than no probe. The quotes
  API's readiness pings the catalog database; this journey stops the actual PostgreSQL
  container of the BDD compose project and watches the endpoint answer unhealthy, then
  brings the database back so the scenarios that follow see a healthy stack.

  Scenario: Quotes API health degrades while the catalog database is down
    Given the distributed application is running
    When the catalog database container is stopped
    Then the quotes API health endpoint reports unhealthy
