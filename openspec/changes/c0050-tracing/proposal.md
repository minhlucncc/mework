## Why

`c0042` lands structured logging + metrics but explicitly defers **distributed tracing**.
Without traces, a request that fans out across the hub (webhook → enqueue → dispatch → runner
→ write-back) and the bus cannot be followed end-to-end, which is the missing leg of
production observability. This change adds OpenTelemetry tracing as a follow-up so the full
"logs + metrics + traces" triad is in place.

## What Changes

- **OpenTelemetry tracing in the hub.** Add OTel SDK setup with an OTLP exporter configured by
  environment (`OTEL_EXPORTER_OTLP_ENDPOINT`, standard OTel env), a no-op/disabled default so
  it's zero-cost when unconfigured, and graceful shutdown flushing.
- **HTTP server spans + context propagation.** Wrap the router with `otelhttp` so each request
  is a span; propagate the trace context (W3C `traceparent`) and stamp the trace id into the
  structured request log (from c0042) so logs and traces cross-link.
- **Span the key internal hops.** Create child spans around the high-value operations —
  webhook verify/parse, enqueue, dispatch publish, write-back — and propagate context across
  the message bus (inject/extract `traceparent` on dispatch messages) so a runner's work links
  back to the originating request.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `observability`: adds **distributed tracing** — OpenTelemetry HTTP server spans, W3C context
  propagation (including across the message bus), trace–log correlation, and an OTLP exporter
  that is disabled/no-op unless configured.

## Impact

- **Server:** OTel setup (tracer provider + OTLP exporter + shutdown) wired in
  `apps/mework-server` and the in-process `apps/mework server start`; `otelhttp` middleware in
  `libs/server/hub/router.go`; spans + bus context propagation in webhook/orchestrator/
  writeback; trace id added to the c0042 request log fields.
- **Dependency:** adds `go.opentelemetry.io/otel` (+ SDK, OTLP exporter, `otelhttp`) to the
  server module.
- **Depends on** `c0042` (the `observability` capability + structured request log to stamp the
  trace id into). Disabled by default → no runtime cost or external dependency unless an OTLP
  endpoint is configured. No schema migration.
