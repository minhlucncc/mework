## Context

c0042 delivers logs + metrics and defers tracing. The hub fans work across HTTP + the message
bus, so end-to-end traces need HTTP spans plus context propagation over the bus.

## Goals / Non-Goals

**Goals:** OTel HTTP spans; W3C propagation incl. across the bus; trace–log correlation;
zero-cost when unconfigured.

**Non-Goals:** tracing every function; a bundled collector (export via OTLP to the operator's
collector); metrics via OTel (Prometheus from c0042 stays the metrics path).

## Decisions

- **OTel SDK + OTLP, disabled by default.** Initialize a tracer provider only when an OTLP
  endpoint is configured; otherwise install a no-op tracer so spans compile away to near-zero
  cost. Flush on shutdown alongside the existing graceful-shutdown path.
- **`otelhttp` at the router edge.** One server-span per request with low-cardinality span
  names (chi route patterns), reusing the same route templating as the c0042 metrics labels.
- **Propagate W3C `traceparent`, including over the bus.** Extract on inbound HTTP; inject into
  dispatch messages published to `runner.<id>.dispatch` so the runner can continue the trace;
  the daemon extracts and spans its run. This is what links a webhook to the runner's work.
- **Trace–log correlation.** Add the active trace id to the c0042 structured request log fields
  so an operator can pivot logs↔traces.

## Risks / Trade-offs

- **[Dependency weight]** → OTel is the standard; isolated to the server module and inert when
  unconfigured.
- **[Bus propagation plumbing]** → keep it to injecting/extracting one header-equivalent field
  on dispatch payload metadata; don't restructure the bus message.
- **[Cardinality]** → span names use route templates, not raw paths (same discipline as
  metrics).

## Migration Plan

Additive and opt-in (no endpoint → no-op). Depends on c0042 being synced (the `observability`
capability + request-log fields). No schema migration.
