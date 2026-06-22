## 1. Push one-shot dispatch on enqueue (TDD)

- [ ] 1.1 Test: enqueuing a webhook-triggered job publishes a dispatch to the assigned runner's
      `runner.<id>.dispatch` topic; the durable job row is still written (dedup/status intact).
- [ ] 1.2 After enqueue, select the runner (reuse c0040 selection) and publish the one-shot
      dispatch; keep the job record authoritative.

## 2. Daemon one-shot dispatch handling (TDD)

- [ ] 2.1 Test: the SSE `Engine` handles a one-shot job dispatch (pull artifact → run → report
      result → ack) without polling.
- [ ] 2.2 Add the one-shot branch to the dispatch worker; remove the poll/claim client call and
      loop.

## 3. Retire the claim route

- [ ] 3.1 Remove `POST /api/v1/jobs/claim` + `claimHandlers` wiring from the router.
- [ ] 3.2 Un-skip the integration subtests: `claim route returns 404` and the SSE publish→ack
      subtest now pass.

## 4. Validation

- [ ] 4.1 `make vet` + `make test` (with `TEST_DATABASE_URL`) green incl.
      `TestMessageBus_PublishSseAckNoRedelivery`.
- [ ] 4.2 `openspec validate c0049-sse-dispatch-migration --strict` passes.
