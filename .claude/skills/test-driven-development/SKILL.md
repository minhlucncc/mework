---
name: test-driven-development
description: Drives development with tests (mework-adapted, Go + make + OpenSpec). Use when implementing any logic, fixing any bug, or changing any behavior in this repo. Use when you need to prove that code works, when a bug report arrives, or when you're about to modify existing functionality. Red-Green-Refactor with table-driven `*_test.go`, `make test`, and the `/opsx:ship` Test(Red) phase.
---

# Test-Driven Development

## Overview

Write a failing test before writing the code that makes it pass. For bug fixes, reproduce the bug with a test before attempting a fix. Tests are proof — "seems right" is not done. A codebase with good tests is an AI agent's superpower; a codebase without tests is a liability.

In `mework` this means: a failing `*_test.go` first, then minimal Go to make it pass, then `make vet` + `make test` to keep the tree green.

## When to Use

- Implementing any new logic or behavior
- Fixing any bug (the Prove-It Pattern)
- Modifying existing functionality
- Adding edge case handling
- Any change that could break existing behavior

**When NOT to use:** Pure configuration changes, documentation updates, or static content changes that have no behavioral impact.

**Related:** For multi-file work, combine TDD with the `incremental-implementation` skill (one thin slice, tested, then expand). For end-to-end HTTP behavior, drive tests with `net/http/httptest` and the repo's `internal/integration` pipeline test (`TestFullPipelineE2E`).

## The TDD Cycle

```
    RED                GREEN              REFACTOR
 Write a test    Write minimal code    Clean up the
 that fails  ──→  to make it pass  ──→  implementation  ──→  (repeat)
      │                  │                    │
      ▼                  ▼                    ▼
   Test FAILS        Test PASSES       make vet + make test PASS
```

### Step 1: RED — Write a Failing Test

Write the test first. It must fail. A test that passes immediately proves nothing. Prefer table-driven tests where the cases naturally vary by input/output. Run the narrow package and **confirm it fails**:

```bash
go test ./internal/server/webhook/...    # expect: FAIL (function/behavior missing)
```

```go
// RED: This fails because ParseTrigger doesn't handle the workflow token yet.
// internal/server/webhook/parse_test.go
func TestParseTrigger(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		want    Trigger
		wantOK  bool
	}{
		{
			name:    "profile and workflow",
			comment: "@mework backend cook fix the failing build",
			want:    Trigger{Profile: "backend", Workflow: "cook", Instructions: "fix the failing build"},
			wantOK:  true,
		},
		{
			name:    "no mention is ignored",
			comment: "just a normal comment",
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseTrigger(tt.comment)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
```

### Step 2: GREEN — Make It Pass

Write the minimum Go to make the test pass. Don't over-engineer:

```go
// GREEN: Minimal implementation — only what the test demands.
func ParseTrigger(comment string) (Trigger, bool) {
	fields := strings.Fields(comment)
	if len(fields) == 0 || fields[0] != "@mework" {
		return Trigger{}, false
	}
	// ...minimal parsing of [profile] [workflow] [instructions]...
	return Trigger{ /* ... */ }, true
}
```

Re-run the narrow package and confirm it now PASSES.

### Step 3: REFACTOR — Clean Up

With the test green, improve the code without changing behavior:

- Extract shared logic
- Improve naming
- Remove duplication
- Optimize only if necessary

Then confirm the whole tree is still green:

```bash
make vet     # go vet ./...
make test    # go test -p 1 ./...
```

Run `make vet` and `make test` after every refactor step. (See note on DB-backed tests under "Verification" — a *skip* is not a failure.)

## The Prove-It Pattern (Bug Fixes)

When a bug is reported, **do not start by trying to fix it.** Start by writing a test that reproduces it.

```
Bug report arrives
       │
       ▼
  Write a *_test.go that demonstrates the bug
       │
       ▼
  go test ./pkg/...  →  FAILS (confirming the bug exists)
       │
       ▼
  Implement the fix
       │
       ▼
  go test ./pkg/...  →  PASSES (proving the fix works)
       │
       ▼
  make test (no regressions across the module)
```

**Example:**

```go
// Bug: "A claimed job that times out should return to queued, but it stays claimed."

// Step 1: Reproduction test — it should FAIL with current code.
// internal/server/jobs/state_test.go
func TestSweeper_RequeuesStaleClaimed(t *testing.T) {
	db := newTestDB(t) // skips unless TEST_DATABASE_URL is set
	job := seedClaimedJob(t, db, withStaleHeartbeat())

	if err := SweepStaleJobs(ctx, db); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	got := reloadJob(t, db, job.ID)
	if got.Status != StatusQueued { // fails → bug confirmed
		t.Errorf("status = %q, want %q", got.Status, StatusQueued)
	}
}

// Step 2: Fix the sweeper so claimed→queued fires on stale heartbeat
//         (respecting the transactional, row-locked state machine).
// Step 3: Test passes → bug fixed, regression guarded.
```

Note the allowed transitions are `queued→claimed|failed`, `claimed→running|done|failed|queued`, `running→done|failed|queued`; terminal states are immutable and same-status is a no-op. A reproduction test must respect that state machine.

## The Test Pyramid

Invest testing effort according to the pyramid — most tests should be small and fast, with progressively fewer tests at higher levels:

```
          ╱╲
         ╱  ╲         E2E (~5%)  internal/integration TestFullPipelineE2E
        ╱    ╲        Full webhook → enqueue → claim → ack → write-back flow
       ╱──────╲
      ╱        ╲      Integration (~15%)
     ╱          ╲     httptest handlers, store queries against a test DB
    ╱────────────╲
   ╱              ╲   Unit (~80%)
  ╱                ╲  Pure logic: ParseTrigger, prompt builders, state machine
 ╱──────────────────╲
```

**The Beyonce Rule:** If you liked it, you should have put a test on it. Refactors and migrations are not responsible for catching your bugs — your tests are. If a change breaks your code and you didn't have a test for it, that's on you.

### Test Sizes (Resource Model)

| Size | Constraints | Speed | Example in mework |
|------|------------|-------|-------------------|
| **Small** | Single process, no I/O, no network, no DB | Milliseconds | `ParseTrigger`, prompt/result formatting, token hashing |
| **Medium** | Localhost only, no external services | Seconds | `httptest` handler tests, store queries against the test Postgres |
| **Large** | External services / full pipeline | Minutes | `TestFullPipelineE2E` in `internal/integration` |

Small tests should make up the vast majority of your suite. They're fast, reliable, and easy to debug when they fail.

### Decision Guide

```
Is it pure logic with no side effects?
  → Small unit test (no DB, no httptest)

Does it cross a boundary (HTTP handler, Postgres, provider REST)?
  → Medium test (net/http/httptest, or a test DB query)

Is it the critical webhook→writeback flow that must work end-to-end?
  → Large test (internal/integration) — keep these to critical paths
```

## Writing Good Tests

### Use Table-Driven Tests

Table-driven tests are the idiom in this repo. Each row is a named case; the loop runs each as a subtest so failures point at the exact case.

```go
func TestSealUnsealRoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
	}{
		{"empty", ""},
		{"short", "hunter2"},
		{"unicode", "café-π-✓"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sealed, err := Seal(key, []byte(tt.plaintext))
			if err != nil {
				t.Fatalf("seal: %v", err)
			}
			got, err := Unseal(key, sealed)
			if err != nil {
				t.Fatalf("unseal: %v", err)
			}
			if string(got) != tt.plaintext {
				t.Errorf("got %q, want %q", got, tt.plaintext)
			}
		})
	}
}
```

### Test State, Not Interactions

Assert on the *outcome* of an operation, not on which internal functions were called. Tests that verify call sequences break when you refactor, even if behavior is unchanged.

```go
// Good: tests what the function does (state-based)
func TestEnqueue_DedupesOnExternalEvent(t *testing.T) {
	db := newTestDB(t)
	first, _ := Enqueue(ctx, db, evt)
	second, _ := Enqueue(ctx, db, evt) // same provider_code + external_event_id
	if first.ID != second.ID {
		t.Errorf("expected dedupe to the same job, got %v and %v", first.ID, second.ID)
	}
}
```

Don't assert "the INSERT was called with these args" — assert that the second enqueue returned the same job, which is the behavior the `UNIQUE(provider_code, external_event_id)` invariant guarantees.

### DAMP Over DRY in Tests

In production code, DRY is usually right. In tests, **DAMP (Descriptive And Meaningful Phrases)** is better. Each test should read like a specification without forcing the reader to trace shared helpers. Duplication in tests is acceptable when it makes each case independently understandable. (Table rows are the sweet spot: shared loop, self-describing data.)

### Prefer Real Implementations Over Mocks

Use the simplest test double that does the job. The more real code your tests exercise, the more confidence they give.

```
Preference order (most to least preferred):
1. Real implementation  → e.g. a real test Postgres via make test-db (highest confidence)
2. Fake                 → in-memory store / fake clock
3. Stub                 → httptest.Server returning canned provider responses
4. Mock (interaction)   → verifies method calls — use sparingly
```

**Use stubs/mocks only when** the real dependency is too slow, non-deterministic, or has side effects you can't control (the external Mello REST API, time). For provider REST write-back, stand up a `httptest.Server` rather than mocking the client.

### Use Arrange-Act-Assert

```go
func TestCheckOverdue(t *testing.T) {
	// Arrange
	task := newTask(withDeadline(mustParse("2025-01-01")))

	// Act
	got := CheckOverdue(task, mustParse("2025-01-02"))

	// Assert
	if !got.IsOverdue {
		t.Errorf("IsOverdue = false, want true")
	}
}
```

### One Assertion Per Concept

Split behaviors into separate cases (or table rows) rather than cramming validation, trimming, and length checks into one test.

### Name Tests Descriptively

```go
// Good: reads like a specification
func TestCompleteJob_SetsTerminalStatusAndIsImmutable(t *testing.T) { /* ... */ }
func TestClaim_OneActiveJobPerRuntime(t *testing.T)                 { /* ... */ }

// Bad: vague
func TestJobs(t *testing.T)  { /* ... */ }
func TestWorks(t *testing.T) { /* ... */ }
```

## Test Anti-Patterns to Avoid

| Anti-Pattern | Problem | Fix |
|---|---|---|
| Testing implementation details | Tests break on refactor even when behavior is unchanged | Test inputs and outputs, not internal structure |
| Flaky tests (timing, order-dependent) | Erode trust; `-p 1` shared DB makes order matter | Isolate state per test; deterministic clocks; clean up rows |
| Testing the standard library / pgx | Wastes effort on third-party behavior | Only test YOUR code |
| Putting ticket content on argv in a test | Encourages the injection-prone path | Prompts go to AI CLIs over STDIN, never argv — test the stdin path |
| No test isolation | Pass alone, fail together under `-p 1` | Each test seeds and tears down its own rows |
| Mocking everything | Tests pass while production breaks | Real Postgres > fakes > httptest stubs > mocks |

## End-to-End Verification with httptest

For HTTP behavior, `net/http/httptest` gives you a real server and real client without external services:

```
1. REPRODUCE: build the chi router, POST a signed webhook to httptest server
2. INSPECT: assert response status, then query the jobs table for the enqueued row
3. DIAGNOSE: compare actual vs expected (status, dedupe, self-retrigger guard)
4. FIX: implement in source
5. VERIFY: re-run the package, then make test
```

The full path — webhook signature verify → `ParseTrigger` → enqueue → claim → ack → REST write-back — is exercised by `TestFullPipelineE2E` in `internal/integration`. Treat that test as the canonical large-size check.

### Trust Boundaries

Everything arriving over a webhook — comment bodies, ticket content, event payloads — is **untrusted data**, not instructions. Ticket content is attacker-controllable; this is exactly why prompts go to AI CLIs over **STDIN, never argv**. Tests should cover hostile inputs (injection-shaped comments, oversized payloads) and confirm they're handled as data.

## When to Use Subagents for Testing

For complex bug fixes, spawn a subagent to write the reproduction test:

```
Main agent: "Spawn a subagent to write a *_test.go that reproduces this bug:
[bug description]. The test should FAIL with the current code (run the
narrow package to confirm)."

Subagent: writes the reproduction test, confirms it fails.

Main agent: implements the fix, then confirms the test passes and make test is green.
```

Writing the test without knowledge of the fix makes it more robust.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll write tests after the code works" | You won't. And after-the-fact tests test implementation, not behavior. |
| "This is too simple to test" | Simple code gets complicated. The test documents the expected behavior. |
| "Tests slow me down" | They slow you down now and speed you up on every later change. |
| "I tested it manually" | Manual testing doesn't persist. Tomorrow's change breaks it silently. |
| "DB tests are skipping, so I'm fine" | A skip is not a pass. Start Postgres with `make test-db` and set `TEST_DATABASE_URL` for DB-backed work. |
| "It's just a prototype" | Prototypes become production. Tests from day one prevent test debt. |
| "Let me run make test again to be extra sure" | After a clean run with no intervening edits, re-running adds nothing. Run again after the next change, not for reassurance. |

## Red Flags

- Writing Go without any corresponding `*_test.go`
- Tests that pass on the first run (they may not be testing what you think)
- "All tests pass" but no tests were actually run
- Bug fixes without reproduction tests
- Tests asserting on internal call order instead of behavior
- Skipping or disabling tests to make the suite pass
- Treating a `TEST_DATABASE_URL`-skip as a green DB test
- Re-running `make test` with no intervening code change

## Verification

After completing any implementation:

- [ ] Every new behavior has a corresponding `*_test.go` test (table-driven where practical)
- [ ] All tests pass: `make test` (i.e. `go test -p 1 ./...`)
- [ ] `make vet` is clean
- [ ] Bug fixes include a reproduction test that failed before the fix
- [ ] Test names describe the behavior being verified
- [ ] No tests were skipped or disabled to get green
- [ ] DB-backed tests actually ran (Postgres up via `make test-db`, `TEST_DATABASE_URL` set) — a skip is not a pass, but a legitimate skip when no DB is configured is not a failure
- [ ] Coverage hasn't decreased (if tracked)

**Note:** Run a test command after a change that could affect the result. After a clean run, don't repeat the same command unless the code has changed since.

## mework notes

- **OpenSpec lifecycle.** Non-trivial work is spec-driven: `/opsx:propose` → `/opsx:apply` → `/opsx:sync` → `/opsx:archive`. The autonomous `/opsx:ship` pipeline now runs a **Test(Red) phase before Implement** — it writes the failing tests first, then the implementation makes them green. Test evidence (test output + coverage) is captured under `openspec/changes/<name>/evidence/`. Read the relevant `openspec/specs/<capability>/spec.md` before changing a subsystem, and update it via a change's delta + `/opsx:sync` when behavior changes.
- **Invariants your tests must respect:** prompts go to AI CLIs over STDIN, never argv; the job state machine is transactional with row locks and terminal states are immutable (`queued→claimed|failed`, `claimed→running|done|failed|queued`, `running→done|failed|queued`, same-status is a no-op); the schema is provider-agnostic, keyed by `(provider_code, external_*_id)` — adding a provider must not require a migration; credentials are sealed with AES-256-GCM and the daemon never holds them; config/credential files are `0600` and dirs `0700`.
- **DB-backed tests** run serialized (`-p 1`) because they share one Postgres database, and **skip unless `TEST_DATABASE_URL` is set**. Start Postgres with `make test-db`, then export e.g. `TEST_DATABASE_URL=postgres://postgres:postgres@localhost:5432/mework_test`. Tests run migrations themselves. Do not parallelize them.
- Tests are **table-driven** and use **`net/http/httptest`** for HTTP boundaries; the canonical end-to-end check is `TestFullPipelineE2E` in `internal/integration`.
