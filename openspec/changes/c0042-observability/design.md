## Context

Logs are unstructured stdlib `log`; there are no metrics. `shared/log.Logger` exists but is
unwired. The recurring constraint is SSE: instrumentation must not buffer or time-bound the
long-lived subscribe/stream responses.

## Goals / Non-Goals

**Goals:** structured, correlated logs via `slog`; a `/metrics` endpoint with runtime + HTTP
metrics; SSE-safe instrumentation.

**Non-Goals:** OpenTelemetry tracing (separate follow-up — exporter/collector choice + span
plumbing is its own effort); log shipping/aggregation infra; per-tenant metric cardinality
explosions (labels limited to route + method + status).

## Decisions

- **`slog` for logging (no new dep).** A small constructor returns a `*slog.Logger` wrapped to
  satisfy `shared/log.Logger`; handler is JSON when `LOG_LEVEL`/env indicates production, text
  otherwise. Replace `chimiddleware.Logger` with a thin slog request-logging middleware that
  reads the chi `RequestID` for correlation. Convert `log.Printf` sites incrementally but
  within this change for the server hot paths.
- **Prometheus for metrics.** Use `client_golang` — the de-facto standard, pull-based, no
  collector required. Register Go runtime + process collectors and an HTTP middleware that
  increments a request counter and observes a latency histogram labeled `method,route,status`.
  Mount `promhttp.Handler()` at `/metrics` (unauthenticated, like `/healthz`; it exposes no
  secrets).
- **SSE-aware HTTP metrics.** The middleware records duration at response completion; for SSE
  that is stream close (open/active gauges optional). It must not wrap the `ResponseWriter` in
  a way that buffers, and must preserve `http.Flusher` so streaming still flushes.

## Risks / Trade-offs

- **[New dependency]** → `client_golang` is widely vetted and standard; isolated to the server
  module. Logging adds no dependency.
- **[Route-label cardinality]** → use chi route patterns (templated, e.g. `/sessions/{id}`),
  not raw paths, to bound label cardinality.
- **[Wrapping ResponseWriter breaks SSE flush]** → use a wrapper that implements `http.Flusher`
  (chi's `WrapResponseWriter` already does); a test asserts SSE still streams.
- **[`/metrics` exposure]** → no secrets in metrics; if needed it can later be bound to an
  internal listener. Documented.

## Migration Plan

Additive. New logger + middleware + `/metrics`; `log.Printf` → structured logger is internal.
No API/schema break. Tracing is a later change.
