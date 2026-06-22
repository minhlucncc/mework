## 1. Implement the World harness (TDD — the suite is the test)

- [ ] 1.1 Implement the `World` verbs against a Postgres-backed `hub.NewServer` behind
      `httptest` (model on `libs/tests/integration`): config/env, `StartHub`/shutdown,
      `Healthz`/`Livez`/`Readyz`, `SeedAccount`, `RegisterRuntime`/enroll, signed webhook post,
      claim/ack, session create/send/stream, assertion helpers. Replace every
      `panic("design-only")`.

## 2. Make scenarios pass (or skip-with-reason)

- [ ] 2.1 Drive the `NN_*` scenarios through the implemented verbs; the ones asserting shipped
      behavior pass.
- [ ] 2.2 Scenarios for deferred/future behavior → `t.Skip` with a reason referencing their
      tracking change (no false-green).

## 3. Activate + CI

- [ ] 3.1 Remove the `//go:build e2e` tag (suite runs under the `TEST_DATABASE_URL` gate;
      skips cleanly without a DB).
- [ ] 3.2 CI: add a Postgres service + `TEST_DATABASE_URL` so the e2e acceptance suite runs.

## 4. Validation

- [ ] 4.1 `make test` with `TEST_DATABASE_URL`: e2e suite runs; shipped-behavior scenarios pass,
      deferred ones skip-with-reason; no panics.
- [ ] 4.2 `make test` without a DB still green (suite skips).
- [ ] 4.3 `openspec validate c0048-e2e-harness --strict` passes.
