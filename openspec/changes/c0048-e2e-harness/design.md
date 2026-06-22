## Context

`libs/tests/e2e` encodes the acceptance scenarios but the `World` harness panics
("design-only"); c0038 tagged it out of the default run. The integration suite already shows
the working pattern (real `hub.NewServer` behind `httptest`, Postgres-gated, migrations +
cleanup), which the harness can adopt.

## Goals / Non-Goals

**Goals:** a real, runnable `World` harness; passing acceptance scenarios for shipped behavior;
the suite wired into CI behind a Postgres service; clean skip without a DB.

**Non-Goals:** asserting not-yet-built behavior as passing (those scenarios are skipped with a
tracked reason, not forced green); load/perf testing; non-Postgres harness backends.

## Decisions

- **Model the harness on the integration tests.** Reuse the proven setup: `store.RunMigrations`
  + `pgxpool` + `hub.NewServer` behind `httptest`, per-suite DB cleanup, signed-webhook helper,
  `meworkclient` for claim/ack and session calls. Implement each `World` verb against that.
- **Honesty over green.** Scenarios that assert shipped behavior must truly pass; scenarios for
  deferred/future behavior (e.g. SSE-push dispatch, claim-route removal) are `t.Skip` with a
  reason referencing their tracking change — never asserted false-green. This mirrors c0038.
- **Drop the build tag + CI Postgres service.** Remove `//go:build e2e`; the suite then runs on
  the normal `TEST_DATABASE_URL` gate. CI adds a Postgres service and sets the env so the
  acceptance gate is real; locally it skips without a DB.
- **Sequence after behavioral changes.** Run after `c0040` (channel) and any SSE-push work so
  the maximum number of scenarios pass rather than skip.

## Risks / Trade-offs

- **[Large harness surface]** → implement verbs incrementally, scenario group by scenario
  group; land with whatever subset passes + the rest skipped-with-reason, then tighten.
- **[CI time/flake with a real DB]** → the integration suite already runs DB-gated in CI; reuse
  its service config and `-p 1` serialization.
- **[Scenarios encode aspirational behavior]** → skip-with-reason + tracking, not false green;
  keeps the gate trustworthy.

## Migration Plan

Test-infra only. Removing the `//go:build e2e` tag activates the suite under the DB gate; CI
gains a Postgres service. No production or schema change.
