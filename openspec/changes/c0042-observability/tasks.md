## 1. Structured logging (TDD)

- [ ] 1.1 Add a `slog`-backed logger satisfying `shared/log.Logger` (JSON/text by env, level
      via `LOG_LEVEL`); unit-test level + format selection.
- [ ] 1.2 Replace `chimiddleware.Logger` with a slog request-logging middleware emitting
      `request_id, method, route, status, duration_ms, bytes`; test that a request produces a
      structured line carrying the chi RequestID.
- [ ] 1.3 Convert server hot-path `log.Printf` call sites (auth, webhook, jobs/orchestrator,
      channel, dispatch, writeback) to the structured logger with stable keys.

## 2. Metrics endpoint (TDD)

- [ ] 2.1 Add `client_golang`; register Go runtime + process collectors.
- [ ] 2.2 HTTP metrics middleware: request counter + latency histogram labeled
      `method,route,status` using chi route patterns; SSE-safe (preserves `http.Flusher`, no
      buffering). Test: `/metrics` exposes the counters; an SSE stream still flushes events.
- [ ] 2.3 Mount `GET /metrics` (promhttp).

## 3. Validation

- [ ] 3.1 `make vet` + `make test` green; `go.sum`/workspace tidy.
- [ ] 3.2 `openspec validate c0042-observability --strict` passes.
