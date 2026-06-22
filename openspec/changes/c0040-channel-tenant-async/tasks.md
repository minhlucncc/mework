## 1. Tenant-scoped worker selection (TDD)

- [ ] 1.1 Unit test: provisioning a channel selects a worker in the tenant that owns
      `(provider_code, resource_id)`, not `DefaultTenantID`.
- [ ] 1.2 Resolve the tenant from the provider connection / watched container in
      `provisioner.go`; pass it to `registrySvc.SelectWorker`. Remove the fixed `tenantID`.
- [ ] 1.3 Update `hub/router.go` construction (no hardcoded tenant).

## 2. Async provisioning off the request path (TDD)

- [ ] 2.1 Unit test: `Router.Route` for an unprovisioned channel returns promptly (does not
      block on the 3×5s retry); provisioning runs in the background.
- [ ] 2.2 Launch provisioning in a background worker (context tied to shutdown); keep retry
      bounded and cancellable.

## 3. Make the current DB suite green (deploy readiness, current behavior only)

- [ ] 3.1 `TestChannelRouting_E2E` passes: a `channel_sessions` row is created and the event is
      delivered on the channel topic (the H4 fix).
- [ ] 3.2 `TestFullPipelineE2E_BehaviorPreservation/self-retrigger` passes: investigate the
      `200`-vs-`202` and the self-retrigger fixture; fix so the current behavior asserts
      correctly (no delivery-model change).
- [ ] 3.3 `TestMessageBus_PublishSseAckNoRedelivery`: align to the **current** poll/claim model
      — the claim route stays (assert current behavior, not 404), and the webhook→SSE-push
      subtest is `t.Skip`ped with a tracked reason (it asserts the future push model). No
      production delivery change.

## 4. Validation

- [ ] 4.1 `make vet` + `make build` green; `make test` (no DB) green.
- [ ] 4.2 `make test` **with** `TEST_DATABASE_URL` green across channel + integration (the full
      current-system suite).
- [ ] 4.3 `openspec validate c0040-channel-tenant-async --strict` passes.
