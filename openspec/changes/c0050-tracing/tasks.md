## 1. OTel setup (TDD)

- [ ] 1.1 Tracer provider + OTLP exporter from env; no-op when unconfigured; shutdown flush.
      Test: unconfigured → no-op tracer (no error, no exporter); configured → provider built.

## 2. HTTP spans + propagation (TDD)

- [ ] 2.1 Wrap the router with `otelhttp`; span names use chi route patterns. Test: a request
      produces a server span and the structured request log carries the trace id.
- [ ] 2.2 Extract inbound W3C context; inject `traceparent` into dispatch messages; daemon
      extracts and spans its run. Test: a dispatch carries the trace context.

## 3. Internal spans

- [ ] 3.1 Child spans around webhook verify/parse, enqueue, dispatch publish, write-back.

## 4. Validation

- [ ] 4.1 `make vet` + `make test` green (tracing inert by default); workspace tidy.
- [ ] 4.2 `openspec validate c0050-tracing --strict` passes.
