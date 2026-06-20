---
name: incremental-implementation
description: Delivers changes incrementally (mework-adapted, Go + make + OpenSpec). Use when implementing any feature or change that touches more than one file in this repo. Use when you're about to write a large amount of code at once, or when a task feels too big to land in one step. Each slice stays buildable (`go build ./...`) and green (`make vet`, `make test`).
---

# Incremental Implementation

## Overview

Build in thin vertical slices — implement one piece, test it, verify it, then expand. Avoid implementing an entire feature in one pass. Each increment should leave the system in a working, testable state. This is the execution discipline that makes large features manageable, and it is exactly how `/opsx:apply` works through an OpenSpec change's `tasks.md`: one task, tested and ticked off, before the next.

## When to Use

- Implementing any multi-file change
- Working through the tasks of an OpenSpec change (`/opsx:apply`)
- Building a new feature from a task breakdown
- Refactoring existing code
- Any time you're tempted to write more than ~100 lines before testing

**When NOT to use:** Single-file, single-function changes where the scope is already minimal.

## The Increment Cycle

```
┌──────────────────────────────────────┐
│                                      │
│   Implement ──→ Test ──→ Verify ──┐  │
│       ▲                           │  │
│       └───── Commit ◄─────────────┘  │
│              │                       │
│              ▼                       │
│          Next slice                  │
│                                      │
└──────────────────────────────────────┘
```

For each slice:

1. **Implement** the smallest complete piece of functionality
2. **Test** — run the narrow package (`go test ./internal/.../...`); write the test first per `test-driven-development`
3. **Verify** — `go build ./...` (or `make build`) succeeds, `make vet` and `make test` stay green
4. **Commit** — save progress with a descriptive message; tick off the matching `tasks.md` item if you're inside an OpenSpec change
5. **Move to the next slice** — carry forward, don't restart

## Slicing Strategies

### Vertical Slices (Preferred)

Build one complete path through the stack. In `mework`, a "stack" is webhook → store → daemon → write-back:

```
Slice 1: ParseTrigger handles a new workflow token   (parse.go + parse_test.go)
    → unit test passes, grammar recognizes the token

Slice 2: Webhook enqueues a job for that trigger      (handler + httptest)
    → handler test passes, row lands in jobs (deduped on provider_code+external_event_id)

Slice 3: Daemon builds the prompt for the workflow    (internal/daemon prompt builder)
    → prompt test passes, content goes over STDIN not argv

Slice 4: Server writes the result back over REST      (jobs write-back + httptest stub)
    → end-to-end path exercised, TestFullPipelineE2E green
```

Each slice delivers working end-to-end functionality.

### Contract-First Slicing

When server and daemon need to evolve in parallel, pin the wire contract first:

```
Slice 0: Define the contract (request/response structs in internal/meworkclient + the server handler signature)
Slice 1a: Implement the server side against the contract + httptest tests
Slice 1b: Implement the daemon/client side against a httptest stub matching the contract
Slice 2: Integrate and exercise the real path via internal/integration
```

### Risk-First Slicing

Tackle the riskiest or most uncertain piece first:

```
Slice 1: Prove the FOR UPDATE SKIP LOCKED claim is correct under one-active-job-per-runtime (highest risk)
Slice 2: Build heartbeat + sweeper requeue on the proven claim
Slice 3: Add the write-back outbox on top
```

If Slice 1 fails, you discover it before investing in the later slices.

## Implementation Rules

### Rule 0: Simplicity First

Before writing any code, ask: "What is the simplest thing that could work?" After writing it, review against these checks — and if the answer is "simplify," reach for the `code-simplification` skill:

- Can this be done in fewer lines?
- Are these abstractions earning their complexity?
- Would a staff engineer say "why didn't you just..."?
- Am I building for hypothetical future requirements, or the current task?

```
SIMPLICITY CHECK:
✗ A generic event-dispatch layer for one webhook provider
✓ A direct adapter under internal/server/provider/<name>/

✗ A reflection-driven config loader for three fields
✓ Three explicit env reads that fail fast

✗ An interface with a single implementation, added "for testing"
✓ The concrete type plus a httptest stub at the boundary
```

Three similar lines is better than a premature abstraction. Implement the naive, obviously-correct version first. Optimize only after correctness is proven with tests.

### Rule 0.5: Scope Discipline

Touch only what the task requires.

Do NOT:
- "Clean up" code adjacent to your change
- Reorder imports in files you're not modifying
- Remove comments you don't fully understand
- Add features not in the spec because they "seem useful"
- Modernize syntax in files you're only reading

If you notice something worth improving outside scope, note it — don't fix it:

```
NOTICED BUT NOT TOUCHING:
- internal/cli/config.go has an unused helper (unrelated to this task)
- The sweeper's log messages could be clearer (separate task)
→ Want me to open a follow-up OpenSpec change for these?
```

### Rule 1: One Thing at a Time

Each increment changes one logical thing. Don't mix concerns.

**Bad:** one commit that adds a provider adapter, refactors the job state machine, and edits a migration.

**Good:** three separate commits — one per change.

### Rule 2: Keep It Compilable

After each increment, `go build ./...` must succeed and existing tests must pass. Don't leave the module in a broken state between slices. Remember the schema is **provider-agnostic** — adding a provider should not require a new migration, so a slice that adds an adapter shouldn't drag a schema change along with it.

### Rule 3: Guard Incomplete Behavior

If a capability isn't ready but you want to land increments, keep it inert behind config rather than half-wired into the live path. Example: a new provider adapter can be registered but not referenced by any active connection, or a behavior can be gated on an env/config flag that defaults off, so the queue and write-back paths stay unchanged until it's complete.

### Rule 4: Safe Defaults

New code defaults to safe, conservative behavior. Honor the invariants:

```go
// Safe: write-back happens server-side with sealed creds unsealed only at write time;
// new options default off so existing jobs are unaffected.
func Enqueue(ctx context.Context, db *pgxpool.Pool, evt Event, opts ...EnqueueOption) (Job, error) {
	o := defaultEnqueueOptions() // conservative defaults
	for _, fn := range opts {
		fn(&o)
	}
	// Respect the self-retrigger guard: never enqueue for a comment authored
	// by the daemon's own provider user.
	// ...
}
```

### Rule 5: Rollback-Friendly

Each increment should be independently revertable:

- Additive changes (new files, new functions, new provider package) are easy to revert
- Modifications to existing code should be minimal and focused
- Goose migrations must have a matching `-- +goose Down` so they can roll back
- Avoid deleting and replacing in the same commit — separate them

## Working with Agents

When directing an agent to implement incrementally:

```
"Let's implement Task 3 from the OpenSpec change.

Start with just the store query and the jobs handler.
Don't touch the daemon yet — that's the next increment.

Write the failing test first (per test-driven-development), then implement.
After implementing, run `go build ./...`, `make vet`, and `make test`
to verify nothing is broken. If DB tests skip, bring up Postgres with
`make test-db` and set TEST_DATABASE_URL."
```

Be explicit about what's in scope and what's NOT in scope for each increment.

## Increment Checklist

After each increment, verify:

- [ ] The change does one thing and does it completely
- [ ] All existing tests still pass (`make test`, i.e. `go test -p 1 ./...`)
- [ ] The build succeeds (`go build ./...` / `make build`)
- [ ] `make vet` (`go vet ./...`) passes
- [ ] The new functionality works as expected (narrow package test green)
- [ ] If inside an OpenSpec change, the matching `tasks.md` item is ticked
- [ ] The change is committed with a descriptive message

**Note:** Run each verification command after a change that could affect it. After a successful run, don't repeat the same command unless the code has changed since. A `TEST_DATABASE_URL`-skip is not a failure, but it's also not coverage — bring up the DB for DB-backed slices.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll test it all at the end" | Bugs compound. A bug in Slice 1 makes Slices 2-4 wrong. Test each slice. |
| "It's faster to do it all at once" | It *feels* faster until something breaks and you can't find which of 500 changed lines caused it. |
| "These changes are too small to commit separately" | Small commits are free. Large commits hide bugs and make rollbacks painful. |
| "I'll wire the new behavior into the live path now and gate it later" | If it isn't complete, keep it inert (unreferenced adapter / flag off) so the queue and write-back stay safe. |
| "This refactor is small enough to include" | Refactors mixed with features make both harder to review and debug. Split them (see `code-simplification`). |
| "I'll skip the down migration for now" | Without `-- +goose Down`, the increment isn't rollback-friendly. Write it now. |
| "Let me run make build again just to be sure" | After a successful run with no intervening edits, re-running adds nothing. Run it again after the next change. |

## Red Flags

- More than 100 lines written without running tests
- Multiple unrelated changes in a single increment
- "Let me just quickly add this too" scope expansion
- Skipping the test/verify step to move faster
- Build or tests broken between increments
- A migration without a matching `-- +goose Down`
- A schema change sneaking into a slice that only adds a provider adapter
- Large uncommitted changes accumulating
- Building abstractions before the third use case demands it
- Touching files outside the task scope "while I'm here"
- Re-running the same build/test command with no intervening code change

## Verification

After completing all increments for a task:

- [ ] Each increment was individually tested and committed
- [ ] The full test suite passes (`make test`)
- [ ] `make build` and `make vet` are clean
- [ ] The feature works end-to-end as specified (exercise `internal/integration` if it touches the pipeline)
- [ ] No uncommitted changes remain

## mework notes

- **OpenSpec lifecycle.** Start non-trivial work with `/opsx:propose`, then implement slice-by-slice with `/opsx:apply` (ticking `tasks.md` as you go), `/opsx:sync` the delta specs into `openspec/specs/`, and `/opsx:archive` when done. The autonomous `/opsx:ship` pipeline applies → verifies → syncs → opens a PR; its Test(Red) phase pairs naturally with thin slices — one failing test, minimal code, repeat (see `test-driven-development`).
- **Invariants to keep intact across slices:** prompts go to AI CLIs over STDIN, never argv; the job state machine is transactional with row locks and terminal states are immutable; the schema is provider-agnostic, keyed by `(provider_code, external_*_id)`, so a new provider goes under `internal/server/provider/<name>/` **without a migration**; credentials are sealed AES-256-GCM and the daemon never holds them; config/credential files are `0600`, dirs `0700`.
- **DB-backed tests** run with `-p 1` (shared Postgres) and **skip unless `TEST_DATABASE_URL` is set** — start Postgres with `make test-db`. Don't parallelize them.
- Prefer **table-driven tests** and **`net/http/httptest`** at HTTP boundaries; the end-to-end slice is validated by `TestFullPipelineE2E` in `internal/integration`.
