## Context

Webhook → job → **poll/claim** is the current model; open-session dispatch already uses SSE
push on `runner.<id>.dispatch` (c0031/c0033). The integration tests assert the unified push
model and the claim route's removal. This change migrates one-shot delivery to push and
retires the poll route.

## Goals / Non-Goals

**Goals:** unify delivery on `runner.<id>.dispatch` (one-shot + session); keep the job record
authoritative; retire the claim route; make the asserted behavior real.

**Non-Goals:** changing job dedup/state-machine/lease semantics (unchanged); removing the
durable job tables (kept as source of truth); multi-runner fan-out of one shot job (still one
assigned runner).

## Decisions

- **Job record stays authoritative; SSE is delivery.** Enqueue still writes the durable job
  (dedup on `(provider_code, external_event_id)`, status machine, lease). After enqueue, a
  dispatch is **pushed** to the selected runner's topic; the runner pulls the artifact, runs,
  and acks via the existing ack path, which advances the job's state machine. The bus retains
  the dispatch (replay/resume) so a momentarily-offline runner still receives it.
- **Runner selection reuses c0040.** Choose the assigned runner via the tenant-scoped worker
  selection introduced in c0040 (one assigned runner per job, honoring one-active-job).
- **Daemon: extend the dispatch loop, delete the poll loop.** The SSE `Engine` already
  processes dispatches; add a one-shot branch (the inverse of the c0033 session branch) that
  pulls the artifact, runs it, reports the result, and acks — then remove the poll/claim
  client call. This unifies the two delivery paths in one loop.
- **Retire `/api/v1/jobs/claim`.** Remove the route + handler so it 404s; the
  `Stateless poll worker` requirement is superseded.

## Risks / Trade-offs

- **[Big behavioral change]** → land after c0040 and gate behind the integration + e2e suites;
  the durable job record means no work is lost if push delivery hiccups (replay on reconnect).
- **[Existing pollers break]** → the daemon is updated in the same change; external pollers (if
  any) must move to SSE — documented as a breaking change for the daemon transport.
- **[Ordering vs at-least-once]** → the job state machine + message ack already give
  at-least-once with idempotent terminal states; reuse them.

## Migration Plan

Behavioral, no schema migration. The daemon and server change together (push replaces poll);
the claim route is removed. Document the transport change. Sequence after c0040; un-skip the
claim-route / SSE-push integration subtests as part of this change.
