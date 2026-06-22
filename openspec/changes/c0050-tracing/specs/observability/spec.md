## ADDED Requirements

### Requirement: Distributed tracing

The server SHALL support distributed tracing via OpenTelemetry: each HTTP request SHALL be a
server span, W3C trace context SHALL be propagated on inbound requests and **across the message
bus** (so a runner's work links back to the originating request), and the active trace id SHALL
be included in the structured request log for trace–log correlation. Tracing SHALL export via
OTLP when an endpoint is configured and SHALL be a no-op (zero external dependency, negligible
cost) when unconfigured.

#### Scenario: Request produces a span correlated with its log

- **WHEN** the server handles a request with tracing configured
- **THEN** it records a server span and the request's structured log carries the trace id

#### Scenario: Trace context propagates across the bus to the runner

- **WHEN** a request publishes a dispatch consumed by a runner
- **THEN** the trace context is carried on the dispatch so the runner's work is part of the
  same trace

#### Scenario: Tracing is inert when unconfigured

- **WHEN** no OTLP endpoint is configured
- **THEN** tracing is a no-op with no exporter and no added external dependency at runtime
