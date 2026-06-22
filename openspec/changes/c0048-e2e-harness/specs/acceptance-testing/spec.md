## ADDED Requirements

### Requirement: Executable end-to-end acceptance suite

The project SHALL provide an **executable** end-to-end acceptance suite that drives a real hub
server (Postgres-backed, in-process behind a test HTTP server) through the system's primary
journeys — server health/liveness/readiness, authentication and runner enrollment, webhook →
job enqueue → claim → ack → write-back, and interactive sessions — with the harness verbs fully
implemented (no `panic`/design-only placeholders). Scenarios that assert shipped behavior SHALL
pass; scenarios for not-yet-built behavior SHALL be explicitly skipped with a tracked reason
rather than asserted as passing. The suite SHALL run in CI against a database service and SHALL
skip cleanly when no test database is configured.

#### Scenario: Acceptance scenarios execute against a real hub

- **WHEN** the acceptance suite runs with a test database configured
- **THEN** the harness starts a real hub and the shipped-behavior scenarios pass (no
  design-only panics)

#### Scenario: Future-behavior scenarios are skipped, not falsely green

- **WHEN** a scenario asserts behavior that is not yet built
- **THEN** it is skipped with a reason referencing its tracking change, rather than reported as
  passing

#### Scenario: Suite is CI-gated and DB-optional locally

- **WHEN** the suite runs in CI (database service present) versus locally without a database
- **THEN** it executes the acceptance gate in CI and skips cleanly when no test database is set
