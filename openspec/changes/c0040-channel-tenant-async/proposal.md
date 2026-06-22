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
