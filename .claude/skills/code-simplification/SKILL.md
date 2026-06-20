---
name: code-simplification
description: Simplifies code for clarity (mework-adapted, Go + make + OpenSpec). Use when refactoring code for clarity without changing behavior in this repo. Use when code works but is harder to read, maintain, or extend than it should be. Use when reviewing code that has accumulated unnecessary complexity. Every change keeps `make vet` and `make test` green and respects the repo's invariants.
---

# Code Simplification

> Inspired by the [Claude Code Simplifier plugin](https://github.com/anthropics/claude-plugins-official/blob/main/plugins/code-simplifier/agents/code-simplifier.md). Adapted here for the `mework` Go project as a process-driven skill.

## Overview

Simplify code by reducing complexity while preserving exact behavior. The goal is not fewer lines — it's code that is easier to read, understand, modify, and debug. Every simplification must pass a simple test: "Would a new team member understand this faster than the original?"

## When to Use

- After a feature is working and tests pass, but the implementation feels heavier than it needs to be
- During code review when readability or complexity issues are flagged
- When you encounter deeply nested logic, long functions, or unclear names
- When refactoring code written under time pressure
- When consolidating related logic scattered across files
- After merging changes that introduced duplication or inconsistency

**When NOT to use:**

- Code is already clean and readable — don't simplify for the sake of it
- You don't understand what the code does yet — comprehend before you simplify
- The code is on a hot path (job claim, write-back loop) and the "simpler" version would be measurably slower
- You're about to rewrite the module entirely — simplifying throwaway code wastes effort

## The Five Principles

### 1. Preserve Behavior Exactly

Don't change what the code does — only how it expresses it. All inputs, outputs, side effects, error behavior, and edge cases must remain identical. In `mework` this is non-negotiable around the **transactional, row-locked job state machine** (terminal states are immutable; allowed transitions only), the **webhook de-dup** on `UNIQUE(provider_code, external_event_id)`, and the **self-retrigger guard**. If you're not sure a simplification preserves behavior, don't make it.

```
ASK BEFORE EVERY CHANGE:
→ Does this produce the same output for every input?
→ Does this maintain the same error behavior (same wrapped errors, same nils)?
→ Does this preserve the same side effects and ordering (DB writes, locks, outbox)?
→ Do all existing tests still pass without modification (make test)?
```

### 2. Follow Project Conventions

Simplification means making code more consistent with the codebase, not imposing external preferences. Before simplifying:

```
1. Read CLAUDE.md and the relevant openspec/specs/<capability>/spec.md
2. Study how neighboring code handles similar patterns
3. Match the project's style for:
   - Error handling (wrapped errors with fmt.Errorf("...: %w", err))
   - Context threading (ctx first parameter)
   - Table-driven test structure
   - Package layout (internal/server/<area>/, provider adapters under provider/<name>/)
   - File perms (0600 files, 0700 dirs) and STDIN-not-argv prompt passing
```

Simplification that breaks project consistency is not simplification — it's churn.

### 3. Prefer Clarity Over Cleverness

Explicit code is better than compact code when the compact version requires a mental pause to parse.

```go
// UNCLEAR: dense nested ternary-style chaining via a helper map literal inline
label := map[bool]string{true: "Admin", false: "User"}[user.IsAdmin]

// CLEAR: a small named function with guard returns
func statusLabel(j Job) string {
	switch j.Status {
	case StatusDone:
		return "Done"
	case StatusFailed:
		return "Failed"
	case StatusRunning:
		return "Running"
	default:
		return "Queued"
	}
}
```

```go
// UNCLEAR: building a map with inline mutation inside a single expression-ish loop
counts := map[string]int{}
for _, j := range jobs { counts[j.RuntimeID] = counts[j.RuntimeID] + 1 }

// CLEAR: a named intermediate and the idiomatic increment
countByRuntime := make(map[string]int, len(jobs))
for _, j := range jobs {
	countByRuntime[j.RuntimeID]++
}
```

### 4. Maintain Balance

Simplification has a failure mode: over-simplification. Watch for these traps:

- **Inlining too aggressively** — removing a helper that gave a concept a name (e.g. `verifySignature`) makes the call site harder to read
- **Combining unrelated logic** — merging two simple functions into one complex function is not simpler
- **Removing "unnecessary" abstraction** — the provider adapter interface exists for provider-agnosticism, not by accident; don't collapse it to inline the Mello path
- **Optimizing for line count** — fewer lines is not the goal; easier comprehension is

### 5. Scope to What Changed

Default to simplifying recently modified code. Avoid drive-by refactors of unrelated code unless explicitly asked to broaden scope (see `incremental-implementation`'s scope discipline). Unscoped simplification creates noise in diffs and risks unintended regressions.

## The Simplification Process

### Step 1: Understand Before Touching (Chesterton's Fence)

Before changing or removing anything, understand why it exists. This is Chesterton's Fence: if you see a fence across a road and don't understand why it's there, don't tear it down. First understand the reason, then decide if it still applies.

```
BEFORE SIMPLIFYING, ANSWER:
- What is this code's responsibility?
- What calls it? What does it call?
- What are the edge cases and error paths?
- Are there tests that define the expected behavior?
- Why might it have been written this way? (Lock ordering? Provider-agnosticism?
  Injection safety — STDIN not argv? File perms?)
- Check git blame / the relevant openspec change: what was the original context?
```

If you can't answer these, you're not ready to simplify. Read more context first. The CLAUDE.md "Conventions & invariants (don't break these)" list is a catalog of fences that look removable but aren't.

### Step 2: Identify Simplification Opportunities

Scan for these patterns — each one is a concrete signal, not a vague smell:

**Structural complexity:**

| Pattern | Signal | Simplification |
|---------|--------|----------------|
| Deep nesting (3+ levels) | Hard to follow control flow | Guard clauses / early returns (idiomatic Go) |
| Long functions (50+ lines) | Multiple responsibilities | Split into focused functions with descriptive names |
| Nested conditional expressions | Requires a mental stack to parse | `switch`, or a lookup map built once |
| Boolean parameter flags | `doThing(true, false)` | Use an options struct (functional options) or separate functions |
| Repeated conditionals | Same `if` check in several places | Extract a well-named predicate |

**Naming and readability:**

| Pattern | Signal | Simplification |
|---------|--------|----------------|
| Generic names | `data`, `res`, `tmp`, `v`, `x` | Rename to the content: `sealedCreds`, `claimedJob` |
| Abbreviated names | `usr`, `cfg`, `evt` | Full words unless universal (`id`, `url`, `db`, `ctx`) |
| Misleading names | `getJob` that also mutates status | Rename to reflect actual behavior (`claimJob`) |
| Comments explaining "what" | `// increment counter` above `count++` | Delete it — the code is clear |
| Comments explaining "why" | `// STDIN, not argv: ticket content is attacker-controllable` | Keep these — they carry intent the code can't express |

**Redundancy:**

| Pattern | Signal | Simplification |
|---------|--------|----------------|
| Duplicated logic | Same 5+ lines in multiple places | Extract a shared function |
| Dead code | Unreachable branches, unused vars, commented-out blocks | Remove (after confirming truly dead; `make vet` helps) |
| Unnecessary wrappers | `func get(id) { return svc.Find(id) }` adding nothing | Inline the wrapper |
| Over-engineered patterns | Factory-for-a-factory, an interface with one impl added "for tests" | Use the concrete type + a httptest stub at the boundary |
| Redundant conversions/assertions | Casting to an already-known type | Remove |

### Step 3: Apply Changes Incrementally

Make one simplification at a time. Run the narrow package, then the suite, after each change. **Submit refactoring changes separately from feature or bug-fix changes.** A PR that refactors and adds a feature is two PRs — split them.

```
FOR EACH SIMPLIFICATION:
1. Make the change
2. go test ./internal/<area>/...   (then make vet + make test)
3. If green → keep going (or commit)
4. If red → revert and reconsider
```

Avoid batching multiple simplifications into a single untested change. If something breaks, you need to know which simplification caused it.

**The Rule of 500:** If a refactoring would touch more than 500 lines, invest in automation (`gofmt -r` rewrite rules, `sed`, or a small AST tool with `go/ast`) rather than editing by hand. Manual edits at that scale are error-prone and exhausting to review.

### Step 4: Verify the Result

After all simplifications, step back and evaluate the whole:

```
COMPARE BEFORE AND AFTER:
- Is the simplified version genuinely easier to understand?
- Did you introduce any pattern inconsistent with the codebase?
- Is the diff clean and reviewable?
- Would a teammate approve this change?
```

If the "simplified" version is harder to understand or review, revert. Not every simplification attempt succeeds.

## Go-Specific Guidance

```go
// SIMPLIFY: pointless context-free wrapper that just forwards
// Before
func getUser(ctx context.Context, id string) (User, error) {
	return userService.FindByID(ctx, id)
}
// After — call userService.FindByID directly; delete the wrapper.

// SIMPLIFY: verbose conditional assignment
// Before
var displayName string
if user.Nickname != "" {
	displayName = user.Nickname
} else {
	displayName = user.FullName
}
// After
displayName := user.Nickname
if displayName == "" {
	displayName = user.FullName
}

// SIMPLIFY: manual slice building
// Before
var active []User
for _, u := range users {
	if u.IsActive {
		active = append(active, u)
	}
}
// After — keep the loop (Go has no stdlib filter); just make intent clear
// with a named predicate if the condition is non-trivial:
active := make([]User, 0, len(users))
for _, u := range users {
	if isActive(u) {
		active = append(active, u)
	}
}

// SIMPLIFY: redundant boolean
// Before
func isValid(s string) bool {
	if len(s) > 0 && len(s) < 100 {
		return true
	}
	return false
}
// After
func isValid(s string) bool {
	return len(s) > 0 && len(s) < 100
}

// SIMPLIFY: deep nesting → guard clauses (idiomatic Go error handling)
// Before
func process(data *Data) error {
	if data != nil {
		if data.Valid() {
			if data.HasPermission() {
				return doWork(data)
			}
			return errors.New("no permission")
		}
		return errors.New("invalid data")
	}
	return errors.New("data is nil")
}
// After
func process(data *Data) error {
	if data == nil {
		return errors.New("data is nil")
	}
	if !data.Valid() {
		return errors.New("invalid data")
	}
	if !data.HasPermission() {
		return errors.New("no permission")
	}
	return doWork(data)
}
```

**Do not "simplify" away these on-purpose patterns:**

- Passing prompts to AI CLIs over **STDIN rather than argv** — looks like an avoidable indirection, but it's the injection-safety fence.
- The **provider adapter interface** even when Mello is the only implementation — it's what keeps the schema provider-agnostic.
- **Wrapped errors** (`%w`) collapsed to bare strings — that loses `errors.Is/As` matching elsewhere.
- Explicit **file perms** (`0600`/`0700`) replaced with defaults.
- Transaction/row-lock scaffolding around state transitions — it enforces the immutable-terminal-state and one-active-job-per-runtime invariants.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "It's working, no need to touch it" | Working code that's hard to read is hard to fix when it breaks. Simplifying now saves time on every future change. |
| "Fewer lines is always simpler" | A one-line nested expression is not simpler than a 5-line `switch`. Simplicity is comprehension speed, not line count. |
| "I'll just quickly simplify this unrelated code too" | Unscoped simplification creates noisy diffs and risks regressions. Stay focused (see `incremental-implementation`). |
| "The types make it self-documenting" | Types document structure, not intent. A well-named function explains *why* better than a signature explains *what*. |
| "This abstraction might be useful later" | Don't preserve speculative abstractions. If it's unused now, remove it and re-add when needed — unless it's a documented invariant fence (provider interface). |
| "The original author must have had a reason" | Maybe. Check git blame and the openspec change — apply Chesterton's Fence. The CLAUDE.md invariants list is the catalog of real reasons. |
| "I'll refactor while adding this feature" | Separate refactoring from feature work. Mixed changes are harder to review, revert, and read in history. |

## Red Flags

- Simplification that requires modifying tests to pass (you likely changed behavior)
- "Simplified" code that is longer and harder to follow than the original
- Renaming things to match preferences rather than project conventions
- Removing error wrapping or error handling because "it's cleaner"
- Collapsing the provider adapter interface or moving prompts onto argv
- Replacing explicit file perms with defaults
- Simplifying code you don't fully understand
- Batching many simplifications into one large, hard-to-review commit
- Refactoring code outside the current task scope without being asked

## Verification

After completing a simplification pass:

- [ ] All existing tests pass without modification (`make test`, i.e. `go test -p 1 ./...`)
- [ ] `make build` / `go build ./...` succeeds with no new warnings
- [ ] `make vet` (`go vet ./...`) passes; code is `gofmt`-clean
- [ ] Each simplification is a reviewable, incremental change
- [ ] The diff is clean — no unrelated changes mixed in
- [ ] Simplified code follows project conventions (checked against CLAUDE.md and the relevant `openspec/specs/`)
- [ ] No error handling or error wrapping was removed or weakened
- [ ] No invariant fence was torn down (STDIN-not-argv, provider-agnostic schema, immutable terminal states, sealed creds, file perms)
- [ ] No dead code left behind (unused imports/vars — `make vet` confirms)
- [ ] A teammate or review agent would approve the change as a net improvement

**Note:** DB-backed tests run with `-p 1` and **skip unless `TEST_DATABASE_URL` is set** (start Postgres with `make test-db`). A skip is not proof your simplification preserved DB behavior — bring up the DB when touching store/jobs code.

## mework notes

- **OpenSpec lifecycle.** A pure simplification rarely needs a new spec, but if it changes observable behavior it does — route it through `/opsx:propose` → `/opsx:apply` → `/opsx:sync` → `/opsx:archive`, and never let a "simplification" silently alter a baselined spec under `openspec/specs/`. Keep refactor commits out of feature/`/opsx:ship` change sets; ship them on their own.
- **Invariant fences (do not simplify away):** prompts go to AI CLIs over STDIN, never argv; the job state machine is transactional with row locks and terminal states are immutable; the schema is provider-agnostic, keyed by `(provider_code, external_*_id)` (the provider adapter interface is load-bearing even with one implementation); credentials are sealed AES-256-GCM and the daemon never holds them; config/credential files are `0600`, dirs `0700`.
- **Verify with the repo's tooling:** `make vet`, `make test`; HTTP behavior is checked via `net/http/httptest` and the end-to-end path via `TestFullPipelineE2E` in `internal/integration`. Pair with `test-driven-development` (a refactor that needs test edits changed behavior) and `incremental-implementation` (one simplification per tested step).
