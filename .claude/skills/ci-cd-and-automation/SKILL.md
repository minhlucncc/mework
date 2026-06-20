---
name: ci-cd-and-automation
description: Automates CI/CD pipeline setup, adapted for the mework Go project. Use when setting up or modifying build and deployment pipelines, the GitHub Actions CI workflow, quality gates, or the /opsx:ship verify step. Use when configuring the Go test runner with a Postgres service container or debugging CI failures.
---

# CI/CD and Automation

## Overview

Automate quality gates so that no change reaches `main` without passing `go build`, `make vet`, and `make test`. CI/CD is the enforcement mechanism for every other skill — it catches what humans and agents miss, and it does so consistently on every single change.

**Shift Left:** Catch problems as early in the pipeline as possible. A bug caught in `make vet` costs seconds; the same bug caught after merge costs hours. Move checks upstream — vet before tests, tests before merge, merge before deploy. In `mework`, `/opsx:ship` runs these same gates *before* it opens the PR, so failures are caught on your machine, not in review.

**Faster is Safer:** Smaller batches and more frequent releases reduce risk, not increase it. One OpenSpec change per PR is easier to debug than a quarter's worth of work. Frequent, small ships build confidence in the release process itself.

## When to Use

- Setting up or modifying the CI pipeline (`.github/workflows/ci.yml`)
- Adding or modifying automated checks
- Configuring the Postgres service container for DB-backed tests
- When a change should trigger automated verification
- Debugging CI failures

## The Quality Gate Pipeline

Every change goes through these gates before merge:

```
Pull Request Opened
    │
    ▼
┌─────────────────────────┐
│   BUILD                  │  go build ./...
│   ↓ pass                 │
│   VET                    │  make vet  (go vet ./...)
│   ↓ pass                 │
│   TEST                   │  make test (go test -p 1 ./...)
│   ↓ pass     ▲ needs Postgres service → TEST_DATABASE_URL
│   (E2E)                  │  TestFullPipelineE2E runs under the same job
└─────────────────────────┘
    │
    ▼
  Ready for review
```

**No gate can be skipped.** If `make vet` fails, fix the code — don't suppress the
diagnostic. If a test fails, fix the code — don't add a `t.Skip`. DB-backed tests
**skip themselves** when `TEST_DATABASE_URL` is unset, so CI *must* provide it or
the most important coverage silently disappears.

## GitHub Actions Configuration

### CI Pipeline (Go + Postgres service)

This is the shape of `.github/workflows/ci.yml` (the workflow file itself is
maintained separately). Tests are serialized (`-p 1`) because they share one
Postgres database; a service container provides `TEST_DATABASE_URL`.

```yaml
# .github/workflows/ci.yml
name: CI

on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

jobs:
  quality:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_DB: mework_test
          POSTGRES_USER: postgres
          POSTGRES_PASSWORD: postgres
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    env:
      TEST_DATABASE_URL: postgres://postgres:postgres@localhost:5432/mework_test
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.7'
          cache: true   # caches the Go module + build cache

      - name: Build
        run: go build ./...

      - name: Vet
        run: make vet

      - name: Test
        run: make test
```

Notes specific to this repo:
- **No migration step is needed in CI.** Tests run the embedded goose migrations
  themselves, and the server runs them automatically on startup. There is no
  separate `migrate` command to invoke.
- `make test` is already `go test -p 1 ./...` — **do not** add `-p` parallelism;
  the shared Postgres DB requires serialized runs.
- The plaintext `postgres/postgres` credential is acceptable for an ephemeral
  CI-only service container, but never reuse it for the server's real
  `SERVER_KEY` / `MEWORK_SECRET_KEY` secrets (those belong in GitHub Secrets).

### Release builds (goreleaser snapshot)

Cross-compiled CLI artifacts come from goreleaser, exposed as `make snapshot`.
A release job (separate from CI) typically runs on a tag:

```yaml
  release:
    runs-on: ubuntu-latest
    if: startsWith(github.ref, 'refs/tags/v')
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.25.7', cache: true }
      - run: make snapshot   # goreleaser cross-compile (CLI only)
```

## Feeding CI Failures Back to Agents

The power of CI with AI agents is the feedback loop. When CI fails:

```
CI fails
    │
    ▼
Copy the failure output
    │
    ▼
Feed it to the agent:
"The CI pipeline failed with this error:
[paste specific error]
Reproduce with `make test` (export TEST_DATABASE_URL first),
fix the root cause, and verify locally before pushing again."
    │
    ▼
Agent fixes → pushes → CI runs again
```

**Key patterns:**

```
go vet failure   → Agent reads the diagnostic location and fixes it
Build error      → Agent checks imports, go.mod, and the failing package
Test failure     → Agent follows the debugging-and-error-recovery skill
DB test "skipped"→ TEST_DATABASE_URL wasn't set — CI service container misconfigured
Race / flake     → Likely the heartbeat/claim row-lock path; investigate, don't re-run blindly
```

To reproduce CI locally:

```bash
make test-db                                            # start docker Postgres
export TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/mework_test
make vet && make test
```

## Deployment & Release Strategy

`mework` ships two binaries (`mework`, `mework-server`). There is no JS preview
deploy. The release flow is:

```
OpenSpec change approved
    │
    ▼
/opsx:ship  → apply → verify (make vet/test) → sync → CHANGELOG → commit → push → PR
    │ STOPS here (no auto-merge)
    ▼
Human review + merge to main
    │
    ▼
/opsx:archive (post-merge)
    │
    ▼
Tag vX.Y.Z → goreleaser snapshot publishes CLI artifacts
    │
    ▼
mework-server deployed; migrations run automatically on startup (embedded goose)
```

### Faster is Safer, applied here

- One change per PR. The `/opsx:ship` verify gate is the same gate CI runs, so
  green-locally usually means green-in-CI.
- The server has no manual migration step to forget — embedded goose runs on
  boot. Keep migrations forward-only and additive where possible
  (the provider-agnostic schema means adding a provider needs **no** migration).

### Rollback

`mework-server` rollback = redeploy the previous binary. Because migrations are
additive and run on startup, prefer migrations that are safe to run against the
previous binary too. CLI rollback = users pin/reinstall a prior goreleaser
artifact.

## Environment Management

```
CI service Postgres   → ephemeral; TEST_DATABASE_URL points at it
SERVER_KEY            → GitHub Secrets / deploy platform vault (never in CI logs)
MEWORK_SECRET_KEY     → GitHub Secrets / deploy platform vault (AES-256-GCM seal key)
DATABASE_URL          → deploy platform (production Postgres DSN)
WEBHOOK_SECRET        → GitHub Secrets / deploy platform vault
```

CI should never hold production secrets. The only secret CI needs is the
throwaway Postgres password for its own service container.

## Automation Beyond CI

### Dependabot

```yaml
# .github/dependabot.yml
version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    schedule:
      interval: weekly
    open-pull-requests-limit: 5
  - package-ecosystem: github-actions
    directory: /
    schedule:
      interval: weekly
```

### Build Cop Role

Designate someone responsible for keeping CI green. When the build breaks, the
Build Cop's job is to fix or revert — not necessarily the person whose change
caused the break. This prevents broken builds from accumulating while everyone
assumes someone else will fix it.

### PR Checks (branch protection)

- **Required status checks:** the `quality` job must pass before merge
- **Required reviews:** at least 1 approval (`/opsx:ship` opens the PR but never merges)
- **Branch protection:** no force-pushes to `main`

## CI Optimization

When the pipeline gets slow, apply these in order of impact:

```
Slow CI pipeline?
├── Cache Go modules + build cache
│   └── setup-go with cache: true (caches GOMODCACHE and the build cache)
├── Keep tests serial but trim DB churn
│   └── Tests share one Postgres DB (-p 1); reduce per-test schema rebuilds, reuse fixtures
├── Split build/vet from the DB test job
│   └── go build + make vet need no Postgres; run them in a parallel job without the service
├── Only run what changed
│   └── Path filters: skip the test job for docs-only / openspec-only PRs
└── Use a larger runner
    └── For CPU-heavy compile, a larger GitHub-hosted runner
```

> Do **not** "optimize" by adding `-p` parallelism to `make test`. The shared
> Postgres database is why it runs `-p 1`; parallelizing corrupts test state.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "CI is too slow" | Cache Go modules and split the no-DB job out. Don't skip gates. |
| "This change is trivial, skip CI" | Trivial changes break builds. CI is fast for trivial changes anyway. |
| "The DB test is flaky, just re-run" | Flakes here usually hide a real claim/heartbeat race. Investigate the row-lock path. |
| "We'll add CI later" | Projects without CI accumulate broken states. Keep the `quality` job green from day one. |
| "I'll just run vet, skip the DB tests" | DB tests are the heart of the job queue. Without `TEST_DATABASE_URL` they silently skip — that's a gap, not a pass. |

## Red Flags

- `TEST_DATABASE_URL` not set in CI → all DB-backed tests silently skip
- `make vet` failures suppressed instead of fixed
- Tests `t.Skip`'d to make the pipeline green
- `-p` parallelism added to `make test`
- Production secrets (`SERVER_KEY`, `MEWORK_SECRET_KEY`) in CI logs or workflow YAML
- A manual "run migrations" step added to CI (they're embedded/automatic)
- No branch protection requiring the `quality` job

## mework notes

- The CI workflow lives at `.github/workflows/ci.yml` and runs `go build ./...`,
  `make vet`, and `make test` with a Postgres **service container** supplying
  `TEST_DATABASE_URL` (DB-backed tests skip without it).
- **`/opsx:ship` runs the same verify gates** (`make vet` / `make test` +
  `openspec validate`) locally before committing, pushing, and opening the PR via
  `gh` — and it **STOPS at the opened PR** (no auto-merge). `/opsx:archive` runs
  after the human merge.
- Migrations are **embedded goose** and run automatically on server startup and
  in tests — there is no separate migrate step to add to CI.
- Capture verification evidence under `openspec/changes/<name>/evidence/`.
- See the sibling `git-workflow-and-versioning` skill for branch/commit/PR
  discipline these gates enforce, and `documentation-and-adrs` for recording
  CI/infra decisions as ADRs or spec updates.

## Verification

After setting up or modifying CI:

- [ ] `go build ./...`, `make vet`, and `make test` all run on every PR and push to `main`
- [ ] The Postgres service container is present and `TEST_DATABASE_URL` is exported to the test step
- [ ] No `-p` parallelism added to `make test`
- [ ] Failures block merge (branch protection requires the `quality` job)
- [ ] No production secrets in the workflow YAML or logs
- [ ] No manual migration step (migrations are embedded/automatic)
- [ ] CI results feed back into the development loop
