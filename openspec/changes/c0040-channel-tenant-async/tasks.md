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

## 3. Integration

- [ ] 3.1 `TestChannelRouting_E2E` passes: a `channel_sessions` row is created and the event is
      delivered on the channel topic.

## 4. Validation

- [ ] 4.1 `make vet` + `make test` (with `TEST_DATABASE_URL`) green for channel + integration.
- [ ] 4.2 `openspec validate c0040-channel-tenant-async --strict` passes.
