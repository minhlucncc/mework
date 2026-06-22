## Context

`bus.Broker` (Publish/Subscribe/Ack + Subscription.Events/Close) is implemented by `memory`
and `postgres`; `nats` is a stub. JetStream provides durable streams, durable consumers,
explicit ack, and subject wildcards — a close match to the existing contract (retained
messages, resume-from-id, ack, `*`/`**` filters).

## Goals / Non-Goals

**Goals:** a NATS JetStream backend at semantic parity with memory/postgres; config-selectable;
contract-tested.

**Non-Goals:** changing the `Broker` interface; multi-region/clustering topology (operational);
migrating existing deployments off postgres (NATS is opt-in).

## Decisions

- **Subjects + stream.** Map dot-topics directly to NATS subjects; provision a JetStream stream
  covering the mework subject space. Filter `*` → NATS `*` (one token), `**` → `>` (rest).
- **Durable consumer per subscriber identity.** `Subscribe(who, filter, fromEventID)` creates/
  binds a durable consumer filtered by subject; `fromEventID` maps to a start sequence / deliver
  policy for resume. `Events()` drains the consumer; `Close()` unsubscribes (durable retained
  for resume).
- **Explicit ack + redelivery.** Messages delivered with ack-explicit; `Ack(msgID)` acks the
  JetStream msg; no-ack within ack-wait → redelivery (matches "no redelivery after ack").
- **Backpressure.** Use max-ack-pending / consumer flow control as the bounded-per-subscriber
  mechanism the contract requires.
- **Event id.** Use the JetStream stream sequence as the monotonic event id the bus exposes.

## Risks / Trade-offs

- **[New external dependency / infra]** → NATS is opt-in via config; memory/postgres remain
  defaults. The dep is isolated to the `nats` package + server module.
- **[Subtle semantic gaps vs the contract]** → mitigated by running the **shared broker
  contract tests** against NATS (gated on a reachable server) so parity is enforced, not
  assumed.
- **[Wildcard mapping edge cases]** → unit-test `*`/`**` ↔ `*`/`>` mapping explicitly.

## Migration Plan

Additive/opt-in. Selecting NATS is `BUS_DRIVER=nats` + `NATS_URL`; absent it, the existing
default backend is used. No schema change.
