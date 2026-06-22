## Context

`AutoProvisioner` selects workers in `p.tenantID` (wired to `registry.DefaultTenantID` in
`hub/router.go`) and `Router.Route` calls `Provision` inline. Webhooks for non-default
tenants never provision, and provisioning latency (3×5s) is borne by the request.

## Goals / Non-Goals

**Goals:** select workers in the event's real tenant; make webhook intake non-blocking;
keep retries bounded and shutdown-aware.

**Non-Goals:** the SSE-push / `runner.<id>.dispatch` delivery model assertions (separate
behavioral pass); a durable provisioning queue (an in-process async worker is sufficient now,
revisit when NATS/durable bus lands in c0045).

## Decisions

- **Tenant resolution source.** Resolve the tenant from the provider connection / watched
  container that the webhook was verified against — the account that owns
  `(provider_code, resource_id)`. Thread that tenant into `SelectWorker`. This matches how the
  rest of the pipeline scopes by `(provider_code, external_*_id)`.
- **Async via a background goroutine (not the request).** `Route` looks up an existing session
  synchronously (fast, cached); on a miss it hands provisioning to a background worker and
  returns. The webhook handler's 202 no longer depends on worker availability.
- **Bounded, cancellable retry.** Keep the 3×5s retry but in the background worker, gated on a
  context tied to server shutdown, so a slow/absent worker never wedges a request and shutdown
  is clean.

## Risks / Trade-offs

- **[Async means the session may not exist when the first event is published]** → the channel
  topic retains messages (memory/postgres bus replay), so the runner receives them once
  provisioned; document the eventual-consistency window.
- **[Tenant resolution lookup adds a query on the cache-miss path]** → only on first event per
  channel; cached thereafter via the channel registry.
- **[In-process async worker lost on restart]** → acceptable now; a durable queue is future
  work (tie to c0045). Log dropped provisioning on shutdown.

## Migration Plan

Additive/behavioral. No schema change. The provisioner constructor drops the fixed tenant
argument; the webhook path becomes non-blocking. Backwards-compatible for the default tenant
(it still resolves correctly).
