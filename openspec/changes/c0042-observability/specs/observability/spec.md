## ADDED Requirements

### Requirement: Structured, correlated logging

The server SHALL emit **structured** logs (via a leveled structured logger) rather than
free-form text, with the level and output format selectable by configuration. Each HTTP
request SHALL produce a structured access log carrying a **correlation id** (the request id)
together with method, route, status, and duration, so logs for one request can be correlated.

#### Scenario: Request produces a structured, correlated log line

- **WHEN** the server handles an HTTP request
- **THEN** it emits a structured log entry containing the request id, method, route, status,
  and duration

#### Scenario: Log level is configurable

- **WHEN** the configured log level is set (e.g. to debug or info)
- **THEN** the logger emits at and above that level and suppresses lower levels

### Requirement: Service metrics endpoint

The server SHALL expose a Prometheus-format metrics endpoint covering process/runtime metrics
and HTTP request metrics (a request counter and a latency histogram labeled by method, route
template, and status). Instrumentation MUST NOT buffer or otherwise impair the long-lived
Server-Sent-Events streams (runtime subscribe, session stream).

#### Scenario: Metrics endpoint exposes request metrics

- **WHEN** requests have been served and a client scrapes the metrics endpoint
- **THEN** it returns Prometheus-format metrics including HTTP request counts/latency and Go
  runtime metrics

#### Scenario: Instrumentation does not impair SSE

- **WHEN** an SSE stream is served while metrics instrumentation is active
- **THEN** the stream continues to flush events to the subscriber (instrumentation records on
  completion, not by buffering)
