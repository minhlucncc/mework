## Why

The entire `libs/tests/e2e` BDD suite (~24 scenario files, `01_*`…`24_*`, plus `api_test.go`,
`bdd_test.go`, `harness_test.go`) is **design-only**: the `World` harness verbs
(`StartHub`/`ConfigBlank`/`Healthz`/`SeedAccount`/`RegisterRuntime`/…) `panic("design-only")`,
so the scenarios cannot run (B2). c0038 gated the package behind `//go:build e2e` to keep
`make test`/CI green, with the explicit follow-up to **build the harness** so the acceptance
scenarios actually execute and CI gains a real end-to-end gate. This change builds it and
removes the tag.

## What Changes

- **Implement the `World` harness verbs** against a real, Postgres-backed `hub.NewServer`
  behind `httptest` (gated on `TEST_DATABASE_URL`): config setup (`ConfigBlank`, env), server
  lifecycle (`StartHub`/shutdown), `Healthz`/`Livez`/`Readyz`, account/identity seeding
  (`SeedAccount`), `RegisterRuntime`/enroll, webhook post (signed), job claim/ack, session
  create/send/stream, and the assertion helpers — replacing every `panic("design-only")` with
  a working implementation.
- **Make the scenarios pass.** Drive the existing `NN_*` scenarios through the implemented
  verbs; fix scenarios that encode aspirational behavior to match the shipped system (or mark
  the genuinely-future ones `t.Skip` with a tracked reason, consistent with the c0038 claim-
  route/SSE-push deferrals).
- **Remove the `//go:build e2e` tag** so the suite runs by default when `TEST_DATABASE_URL` is
  set, and **wire it into CI** with a Postgres service so the acceptance gate is real. Without
  a DB it skips cleanly (like the integration suite).

## Capabilities

### New Capabilities
- `acceptance-testing`: an executable BDD acceptance suite drives the real hub end-to-end
  (server lifecycle, auth/enroll, webhook→job→ack, sessions/chat) and runs in CI against a
  Postgres service, replacing the design-only scaffolding.

### Modified Capabilities
<!-- none -->

## Impact

- **Tests/harness:** `libs/tests/e2e/harness_test.go` + `World` verbs (implement),
  `libs/tests/e2e/*_test.go` (drive/adjust), remove the `//go:build e2e` tag added by c0038.
- **CI:** `.github/workflows/ci.yml` runs the e2e suite with a Postgres service + the e2e
  build (the tag removal means it runs under the normal DB-gated path).
- **Depends on** the behavioral changes that make scenarios real where they assert shipped
  behavior (notably `c0040` for channel routing; some scenarios may remain skipped pending the
  SSE-push migration — tracked). Best sequenced after the hardening + behavioral changes.
- No production code change; no schema migration.
