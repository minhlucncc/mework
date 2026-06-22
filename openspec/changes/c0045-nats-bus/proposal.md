## Why

The message bus has a pluggable `Broker` interface with working `memory` and `postgres`
backends, but the **NATS JetStream** backend is a stub (`libs/server/bus/nats/bus.go` — an
`init()` that prints "nats bus stub registered", no implementation). The in-memory broker is
single-process and the Postgres broker, while durable, polls/notifies through the DB. For a
horizontally-scaled, low-latency, durable bus, JetStream is the intended production backend
(M1). This change implements it behind the existing interface so it's a config switch, not a
code change elsewhere.

## What Changes

- **Implement the NATS JetStream broker** satisfying `bus.Broker` (Publish / Subscribe /
  Ack) with the same semantics the `memory`/`postgres` backends provide:
  - topic → JetStream subject mapping (hierarchical dot topics map naturally to NATS subjects;
    the `*`/`**` filter wildcards map to NATS `*`/`>`);
  - **durable, resumable** delivery (a durable consumer per subscriber identity; resume from a
    last-delivered sequence ≈ the bus `fromEventID`);
  - **explicit ack** (Ack maps to JetStream message ack; redelivery on no-ack);
  - bounded per-subscriber backpressure (consumer flow control / max-ack-pending).
- **Register the backend** so selecting it is configuration (`BUS_DRIVER=nats` +
  `NATS_URL`), blank-imported in the server main like the postgres backend.
- **Contract parity tests** — run the shared broker contract against the NATS backend (gated
  on a reachable NATS, skip otherwise, mirroring the Postgres test gating).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `message-bus`: adds a **NATS JetStream broker** implementation of the pluggable broker
  contract — durable, resumable, explicitly-acked, wildcard-filtered delivery — selectable by
  configuration, at parity with the existing backends.

## Impact

- **Server:** `libs/server/bus/nats/bus.go` (real implementation), backend registration +
  `apps/mework-server` blank-import; config (`BUS_DRIVER`, `NATS_URL`).
- **Dependency:** adds `github.com/nats-io/nats.go` (+ JetStream) to the server module.
- **Tests:** the shared broker contract suite parameterized over the NATS backend, gated on
  `NATS_URL`/a reachable server (skips cleanly when unset — like `TEST_DATABASE_URL`).
- Default backend is unchanged (memory in tests, configurable in prod); NATS is opt-in. No
  schema migration.
