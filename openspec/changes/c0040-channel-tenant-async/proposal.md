## Why

The channel auto-provisioner (`libs/server/channel/provisioner.go`) — which stands up a
session+worker when a webhook arrives for an unprovisioned channel — has two
production-blocking flaws found in the assessment:

- **H4 (correctness):** worker selection is hardcoded to `registry.DefaultTenantID`
  (`selectWorkerWithRetry`, `provisioner.go:87`). A runner enrolled in any other tenant is
  never selected, so `SelectWorker` returns `ErrNotFound` and provisioning fails for every
  non-default tenant (verified by `TestChannelRouting_E2E`: "Auto-provision failed … runtime
  not found").
- **H5 (DoS/latency):** provisioning runs **synchronously inside the webhook request**, with
  a 3× 5s retry. A webhook for an unprovisionable channel pins the request goroutine for
  ~10s (verified) and returns 202 only after; a flood exhausts request goroutines.

## What Changes

- **Derive the tenant from the event, not a constant.** The auto-provisioner resolves the
  tenant for `(provider_code, resource_id)` from the owning provider connection / watched
  container (the same account that the webhook was verified against) and selects a worker in
  **that** tenant. `DefaultTenantID` is removed as the selection scope.
- **Move provisioning off the request path.** `channel.Router.Route` enqueues/launches
  provisioning asynchronously and returns promptly; the webhook is acknowledged (202) without
  waiting for worker selection + retries. Provisioning failures are logged (and surfaced via
  channel status), not blocked on.
- **Bounded async retry.** The retry/backoff runs in the background worker with a cap and
  respects shutdown; it never holds an inbound request.

## Deploy-readiness (also in scope)

This is the single "make the current system deploy-ready" change — fix the real bugs in the
**current** features and make the database-backed test suite green for the **current**
poll/queue model. No new features. Specifically, in addition to the channel fix:

- **Make `make test` (with a database) green for current behavior.** The DB-backed integration
  tests currently fail in three ways: (1) `TestChannelRouting_E2E` — the H4 bug fixed here;
  (2) `TestFullPipelineE2E_BehaviorPreservation/self-retrigger` — current behavior, fix the
  fixture/assertion so it passes; (3) `TestMessageBus_PublishSseAckNoRedelivery` — its
  "claim route returns 404" and webhook→`runner.<id>.dispatch` push subtests assert a
  **future** SSE-push model. The current, shipped model is poll/claim (per CLAUDE.md), so
  these subtests are **aligned to current behavior** (the claim route stays; the push subtest
  is skipped with a tracked reason) rather than migrating delivery. No delivery-model change.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `channel-routing`: auto-provisioning selects a worker in the **tenant derived from the
  event's connection** (not a fixed default tenant), and runs **asynchronously** off the
  webhook request path.
- `webhook-pipeline`: webhook intake acknowledges promptly and does not block on
  channel auto-provisioning (worker selection + retries happen in the background).

## Impact

- **Server:** `libs/server/channel/provisioner.go` (tenant resolution; drop the fixed
  `tenantID`), `libs/server/channel/router.go` (async dispatch of provisioning),
  `libs/server/hub/router.go` (provisioner construction no longer passes a hardcoded tenant).
- **Tests:** `libs/tests/integration/pipeline_test.go::TestChannelRouting_E2E` should pass
  (channel_sessions row created, event delivered) once tenant scoping + async land; add a unit
  test for tenant resolution and for non-blocking Route.
- **Depends on** nothing new; unblocks the channel-routing E2E that c0038 deferred.
- Preserves the c0027 boundary and provider-agnostic schema. No migration.
