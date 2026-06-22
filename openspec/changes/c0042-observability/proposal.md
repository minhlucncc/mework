## Why

The assessment (M6) found the server has **no metrics, no tracing, and mostly unstructured
`log.Printf`** (only the channel sweeper uses `slog`). For production operability we need
structured, queryable logs and basic service metrics. A shared logging interface already
exists (`libs/shared/log.Logger`) but is unused — the binaries log via stdlib `log`.

## What Changes

- **Structured logging (stdlib `slog`, dependency-free).** Provide a server logger built on
  `log/slog` (JSON in production, text in dev; level via `LOG_LEVEL`) that satisfies the
  existing `shared/log.Logger` interface. Replace the chi `Logger` middleware with a
  slog-based **request logger** emitting structured fields (request id, method, path, status,
  duration, bytes). Convert the server's hot-path `log.Printf` call sites (auth, webhook,
  jobs, channel, dispatch, writeback) to the structured logger with stable keys. Each request
  log carries the chi `RequestID` so lines correlate.
- **Service metrics (`/metrics`, Prometheus exposition).** Expose Go runtime metrics plus HTTP
  request counters/latency histograms (labeled by route + status) via a metrics middleware and
  a `/metrics` endpoint. The middleware is **SSE-aware** — it records on stream *open/close*
  and does not buffer or block long-lived `/jobs/subscribe` and `/sessions/{id}/stream`
  responses.

## Capabilities

### New Capabilities
- `observability`: structured request/server logging with correlation, and a Prometheus
  metrics endpoint covering runtime + HTTP request metrics, without affecting SSE streams.

### Modified Capabilities
<!-- none -->

## Impact

- **Shared:** a concrete `slog`-backed implementation of `shared/log.Logger`
  (`libs/shared/log`).
- **Server:** `libs/server/hub/router.go` (slog request logger + metrics middleware +
  `/metrics`), and conversion of `log.Printf` call sites across `libs/server/*` to structured
  logging.
- **Dependency:** adds `github.com/prometheus/client_golang` to the server module for the
  metrics exposition (runtime via `promhttp`/`collectors`). Logging stays stdlib-only.
- **Non-goals (deferred):** distributed **tracing** (OpenTelemetry) — larger, needs an
  exporter/collector decision; tracked as a follow-up. This change lands logging + metrics.
- No schema migration. SSE behavior preserved.
