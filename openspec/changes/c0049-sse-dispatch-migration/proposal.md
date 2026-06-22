## Why

The system has two delivery models living side by side. The **current** webhook pipeline
enqueues a job that the daemon **polls** for via `POST /api/v1/jobs/claim` (poll/queue). The
**target** agent-hub model pushes work to the assigned runner over SSE on
`runner.<id>.dispatch` (already used for open-session dispatch in c0031/c0033). The
integration suite asserts the target — `TestMessageBus_PublishSseAckNoRedelivery` expects a
webhook to surface as a dispatch event on `runner.<id>.dispatch`, and a subtest asserts the
legacy `claim` route is gone (H2). Today neither holds: the webhook enqueues a job, and the
claim route still serves 204. This is the one remaining behavioral gap keeping the
message-bus E2E (and parts of c0048) red.

## What Changes

- **Push webhook-triggered work to the assigned runner over SSE.** On a verified, deduplicated
  trigger, publish a dispatch to `runner.<id>.dispatch` for the selected runner (reusing the
  channel auto-provision worker selection from `c0040`), so the daemon receives work by push
  instead of polling. The job record remains the durable source of truth (status/dedup/lease);
  the SSE dispatch is the delivery mechanism, acked via the existing message-ack path.
- **Retire the legacy poll/claim route.** Remove `POST /api/v1/jobs/claim` (and the
  `claimHandlers` wiring) once the daemon's SSE dispatch loop drives one-shot jobs, so the
  route returns 404/410 as the tests assert. The daemon's `Stateless poll worker` behavior is
  superseded by the SSE dispatch loop (already present for sessions; extended to one-shot
  jobs).
- **Daemon: one-shot jobs via the dispatch loop.** The runner's SSE `Engine` handles one-shot
  job dispatches on `runner.<id>.dispatch` (pull artifact / run / ack) in addition to the
  open-session dispatches it already handles, removing the poll loop.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `daemon-runtime`: the daemon receives **one-shot work by SSE push** on
  `runner.<id>.dispatch` (superseding the stateless poll/claim worker), unifying one-shot and
  open-session delivery on the dispatch stream.
- `job-queue`: webhook-triggered jobs are **delivered by push** to the assigned runner; the
  durable job record remains the source of truth, and the legacy `claim` poll route is
  retired.

## Impact

- **Server:** `libs/server/hub/router.go` (remove the `/api/v1/jobs/claim` route),
  `libs/server/orchestrator` (claim handler retired; dispatch-on-enqueue for the selected
  runner), webhook/channel enqueue path publishes the runner dispatch.
- **Client/daemon:** `libs/client/runner/engine.go` + `dispatch.go` (handle one-shot job
  dispatches on the dispatch topic; remove the poll loop), `libs/client/subscribe` (drop the
  claim call).
- **Tests:** `TestMessageBus_PublishSseAckNoRedelivery` (publish→SSE→ack and claim-route-gone)
  passes; the c0038-skipped claim-route subtest and the SSE-push subtest are un-skipped.
- **Depends on** `c0040` (tenant-scoped worker selection) for choosing the runner to push to.
  Significant behavioral change — sequence carefully, after `c0040`. No schema migration (the
  job tables are unchanged; only delivery moves from pull to push).
