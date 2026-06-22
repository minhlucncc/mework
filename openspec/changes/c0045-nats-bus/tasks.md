## 1. NATS JetStream broker (TDD)

- [ ] 1.1 Unit-test the topic↔subject and filter (`*`/`**` ↔ `*`/`>`) mapping.
- [ ] 1.2 Implement `bus.Broker` over JetStream: Publish (stream seq = event id), Subscribe
      (durable consumer per identity, resume from `fromEventID`), Ack (explicit), bounded
      backpressure (max-ack-pending).
- [ ] 1.3 Register the backend; blank-import in `apps/mework-server`; config `BUS_DRIVER=nats`
      + `NATS_URL`.

## 2. Contract parity (TDD)

- [ ] 2.1 Run the shared broker contract suite against the NATS backend, gated on `NATS_URL`/a
      reachable server (skip cleanly when unset). Assert publish→subscribe, resume-from-id,
      ack→no-redelivery, wildcard filters, and per-subscriber backpressure.

## 3. Validation

- [ ] 3.1 `make vet` + `make test` green (NATS tests skip without `NATS_URL`); workspace tidy.
- [ ] 3.2 `openspec validate c0045-nats-bus --strict` passes.
