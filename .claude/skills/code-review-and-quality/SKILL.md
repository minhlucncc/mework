---
name: code-review-and-quality
description: Conducts multi-axis code review (mework-adapted, Go). Use before merging any change. Use when reviewing Go code written by yourself, another agent, or a human. Use when you need to assess code quality across correctness, readability, architecture, security, and performance before it enters main. Complements the built-in `/code-review` and `/security-review` slash commands and the `/opsx:address-review` PR-comment loop.
---

# Code Review and Quality

## Overview

Multi-dimensional code review with quality gates. Every change gets reviewed before merge — no exceptions. Review covers five axes: correctness, readability, architecture, security, and performance.

This skill is the methodology behind, and complements, the repo's automation:
- the built-in **`/code-review`** slash command (diff-focused correctness + cleanup pass),
- the built-in **`/security-review`** slash command (security-focused pass — see the `security-and-hardening` sibling skill for the controls),
- the **`/opsx:address-review`** loop, which applies PR-comment feedback against an OpenSpec change.

Use those for the mechanics; use this skill for the standard you hold the change to.

**The approval standard:** Approve a change when it definitely improves overall code health, even if it isn't perfect. Perfect code doesn't exist — the goal is continuous improvement. Don't block a change because it isn't exactly how you would have written it. If it improves the codebase, follows the project's Go conventions, and upholds the invariants in CLAUDE.md, approve it.

## When to Use

- Before merging any PR or change (including before `/opsx:ship` opens a PR)
- After completing a feature implementation via `/opsx:apply`
- When another agent or model produced Go code you need to evaluate
- When refactoring existing packages
- After any bug fix (review both the fix and the regression test — see `debugging-and-error-recovery`)

## The Five-Axis Review

Every review evaluates code across these dimensions:

### 1. Correctness

Does the code do what it claims to do?

- Does it match the OpenSpec delta / task requirements?
- Are edge cases handled (nil, empty slice/map, zero values, boundary values)?
- Are error paths handled? Every returned `error` checked, wrapped with `%w` where the caller needs to inspect it, not swallowed.
- Does it pass `make test`? Did the DB-backed tests actually run (`TEST_DATABASE_URL` set), or silently skip?
- Are there off-by-one errors, **race conditions**, or state inconsistencies? Concurrency lives in the poll loop, heartbeat, claim path, and sweeper — check with `go test -race`.
- Does it preserve the job state-machine invariants (only allowed transitions, terminal states immutable)?

### 2. Readability & Simplicity

Can another engineer (or agent) understand this code without the author explaining it?

- Are names idiomatic Go and consistent with the package? (Short receiver names; exported identifiers documented; no `temp`, `data`, `result` without context; no stutter like `jobs.JobJob`.)
- Is the control flow straightforward? Prefer early returns and guard clauses over deep nesting. `if err != nil { return ... }` at each step, not a pyramid.
- Is the code organized logically (related code grouped, clear package boundaries under `internal/`)?
- Are there "clever" tricks (reflection, unsafe, channel gymnastics) that should be simplified?
- **Could this be done in fewer lines?** (1000 lines where 100 suffice is a failure.)
- **Are abstractions earning their complexity?** Don't add a new provider interface method or a generic helper until a second/third caller needs it. The provider-agnostic schema is the one deliberate generalization — match its level, don't out-abstract it.
- Would a doc comment clarify non-obvious intent? (But don't comment obvious code. Exported funcs get a comment starting with the name.)
- Any dead code: unused exported funcs, `_ = x` no-ops, backwards-compat shims, `// removed` comments?

### 3. Architecture

Does the change fit the system's design?

- Does it follow existing patterns or introduce a new one? If new, is it justified?
- Does it respect the `internal/` package boundaries (CLI/daemon vs server vs store vs provider adapters)? Does the dependency flow point the right way — adapters depend on the provider interface, not vice versa?
- Is there code duplication that should be shared?
- Does a new provider go under `internal/server/provider/<name>/` **without** requiring a migration (provider-agnostic schema, keyed by `(provider_code, external_*_id)`)? If a "new provider" needs a schema change, the abstraction is wrong.
- Is the abstraction level appropriate (not over-engineered, not too coupled)?
- Does the change belong to an OpenSpec change? Non-trivial behavior changes should have a spec delta, not just code.

### 4. Security

For detailed security guidance, see the `security-and-hardening` sibling skill and the built-in `/security-review` command. In Go review specifically, check:

- Is external input (webhook payloads, ticket content, CLI flags) validated at the boundary before use?
- **Prompts to AI CLIs go over STDIN, never argv** (ticket content is attacker-controllable — passing it as an argument is command injection). See `internal/agentrun/runner.go`.
- Are SQL/pgx queries parameterized (`$1`, `$2`), never string-concatenated?
- Are secrets kept out of code, logs, and version control? No `rt_token`, PAT, AES key, or unsealed credential in log lines or error strings.
- Are credentials sealed (AES-256-GCM) at rest and unsealed only server-side at write-back? The daemon must never hold provider credentials.
- Is the webhook signature verified before the payload is trusted? Is `rt_token` looked up via HMAC-SHA256, never compared in plaintext?
- Are auth checks correct — PAT on `/api/v1` management routes, `rt_token` on `/api/v1/jobs/*`?
- Is data from external sources (provider APIs, webhooks, ticket content, config files) treated as untrusted?

### 5. Performance

Does the change introduce performance problems?

- Any N+1 query patterns against Postgres? Prefer a single query / `IN` over a loop of queries.
- Any unbounded loops or unconstrained data fetching? List/claim queries should be bounded and use the existing `FOR UPDATE SKIP LOCKED` claim path.
- Any blocking call inside the poll loop or an HTTP handler that should have a timeout/context (the 30m run cap, 30s heartbeat, client timeouts)?
- Any goroutine leaks — a goroutine started without a way to stop it, or a channel never drained?
- Any large allocation in a hot path (per-poll, per-heartbeat)?
- Any missing pagination on list endpoints?

## Change Sizing

Small, focused changes are easier to review, faster to merge, and safer to deploy. Target these sizes:

```
~100 lines changed   → Good. Reviewable in one sitting.
~300 lines changed   → Acceptable if it's a single logical change.
~1000 lines changed  → Too large. Split it.
```

A single OpenSpec change should ideally map to a reviewable diff. If a change's
tasks add up to a 1000-line diff, the change itself was probably too big — split
the proposal.

**What counts as "one change":** A single self-contained modification that
addresses one thing, includes related `*_test.go` coverage, and keeps `make
build` / `make test` green after submission. One part of a feature — not the whole
feature.

**Splitting strategies when a change is too large:**

| Strategy | How | When |
|----------|-----|------|
| **Stack** | Submit a small change, start the next based on it | Sequential dependencies |
| **By package** | Separate changes for `internal/server/*` vs `internal/daemon` etc. | Cross-cutting concerns |
| **Horizontal** | Add the shared interface/migration first, then consumers | Layered work (e.g. new provider adapter) |
| **Vertical** | Break into smaller full-pipeline slices | Feature work (webhook → jobs → write-back) |

**When large changes are acceptable:** Complete file deletions and automated
refactoring (e.g. `gofmt`/rename) where the reviewer only needs to verify intent,
not every line. A new goose migration plus its generated touchpoints can be large
but is still one logical change.

**Separate refactoring from feature work.** A change that refactors an existing
package and adds new behavior is two changes — submit them separately. Small
cleanups (a rename, a `gofmt`) can be included at reviewer discretion.

## Change Descriptions

Every change needs a description that stands alone in version-control history.

**First line:** Short, imperative, standalone. "Dedupe webhook events on
external_event_id" not "Deduping webhook events." Informative enough that someone
searching history understands it without the diff.

**Body:** What is changing and why. Include context, decisions, and reasoning not
visible in the code. Link to the OpenSpec change name, the ticket, benchmark
results, or design notes where relevant. Acknowledge approach shortcomings when
they exist.

**Anti-patterns:** "Fix bug," "Fix build," "Add patch," "Moving code from A to B,"
"Phase 1," "Add convenience functions."

End commit messages with the required co-author trailer
(`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`).

## Review Process

### Step 1: Understand the Context

Before looking at code, understand the intent:

```
- What is this change trying to accomplish?
- Which OpenSpec change / spec delta does it implement?
- What is the expected behavior change?
```

### Step 2: Review the Tests First

Tests reveal intent and coverage:

```
- Do tests exist (table-driven where practical)?
- Do they test behavior (not implementation details)?
- Are edge cases covered (nil, empty, error paths, boundary)?
- Do tests have descriptive names (TestEnqueueDedupesOnExternalEventID)?
- HTTP-layer behavior driven via net/http/httptest? Pipeline behavior via internal/integration?
- Would the tests catch a regression if the code changed?
- Do the DB-backed tests run under -p 1, and skip cleanly without TEST_DATABASE_URL?
```

### Step 3: Review the Implementation

Walk through the code with the five axes in mind:

```
For each file changed:
1. Correctness: Does this code do what the test says it should? Errors checked? Invariants held?
2. Readability: Idiomatic Go? Can I understand it without help?
3. Architecture: Does it fit the internal/ package boundaries and the provider-agnostic design?
4. Security: stdin-not-argv, parameterized queries, no secrets in logs, auth/token checks correct?
5. Performance: N+1, unbounded queries, goroutine leaks, missing timeouts?
```

### Step 4: Categorize Findings

Label every comment with its severity so the author knows what's required vs optional:

| Prefix | Meaning | Author Action |
|--------|---------|---------------|
| *(no prefix)* | Required change | Must address before merge |
| **Critical:** | Blocks merge | Security hole (e.g. prompt via argv, plaintext token compare), data loss, broken state machine |
| **Nit:** | Minor, optional | Author may ignore — `gofmt`-clean style, naming preference |
| **Optional:** / **Consider:** | Suggestion | Worth considering, not required |
| **FYI** | Informational only | No action needed — context for the future |

This prevents authors from treating all feedback as mandatory.

### Step 5: Verify the Verification

Check the author's verification story:

```
- Did make vet / make build / make test pass?
- Did the DB-backed tests actually run (TEST_DATABASE_URL set), or skip?
- Was the change exercised through internal/integration if it touches the pipeline?
- Is there a before/after for performance-relevant changes?
```

## Multi-Model / Multi-Agent Review Pattern

Use different agents for different review perspectives:

```
Agent A writes the code (e.g. via /opsx:apply)
    │
    ▼
Agent B reviews for correctness and architecture (this skill / /code-review)
    │
    ▼
Agent A addresses the feedback (/opsx:address-review)
    │
    ▼
Human makes the final call
```

Different models have different blind spots. AI-generated Go often compiles and
looks plausible while ignoring an unchecked error, a leaked goroutine, or a broken
invariant — review it harder, not softer.

**Example prompt for a review agent:**
```
Review this Go change for correctness, security, and our CLAUDE.md invariants.
The OpenSpec change is [X]. The change should [Y]. Confirm prompts still go to
the AI CLI over stdin, queries are parameterized, and the job state machine's
allowed transitions are preserved. Flag issues as Critical / Important / Suggestion.
```

## Dead Code Hygiene

After any refactoring or implementation change, check for orphaned code:

1. Identify code now unreachable or unused (`go vet`, `staticcheck`/`unused` if available, or grep for the symbol).
2. List it explicitly.
3. **Ask before deleting:** "Should I remove these now-unused elements: [list]?"

Don't leave dead code lying around — it confuses future readers and agents. But don't silently delete things you're unsure about.

```
DEAD CODE IDENTIFIED:
- formatLegacyResult() in internal/daemon/result.go — replaced by formatResult()
- legacyTriggerKeyword handling in internal/server/webhook/parse.go — superseded by @mework grammar
- unused MELLO_LEGACY_URL constant in internal/cli/config.go — no remaining references
→ Safe to remove these?
```

## Review Speed

Slow reviews block the whole flow. The cost of context-switching to review is less than the waiting cost imposed on others.

- **Respond within one business day** — maximum, not target.
- **Ideal cadence:** respond shortly after a review request arrives, unless deep in focused coding.
- **Prioritize fast individual responses** over quick final approval. Quick feedback reduces frustration even across multiple rounds.
- **Large changes:** ask the author to split them rather than reviewing one massive changeset.

## Handling Disagreements

When resolving review disputes, apply this hierarchy:

1. **Technical facts and data** override opinions and preferences.
2. **Go style** (`gofmt`, Effective Go, the standard library's conventions) is the authority on style matters.
3. **The CLAUDE.md invariants** are non-negotiable — a change that breaks one doesn't merge regardless of preference.
4. **Codebase consistency** is acceptable if it doesn't degrade overall health.

**Don't accept "I'll clean it up later."** Deferred cleanup rarely happens. Require cleanup before submission unless it's a genuine emergency; otherwise file a tracking ticket with self-assignment.

## Honesty in Review

When reviewing code — yours, another agent's, or a human's:

- **Don't rubber-stamp.** "LGTM" without evidence of review helps no one.
- **Don't soften real issues.** Calling a guaranteed nil deref "a minor concern" is dishonest.
- **Quantify problems when possible.** "This N+1 adds one round-trip per job in the claim loop" beats "this could be slow."
- **Push back on approaches with clear problems.** Sycophancy is a failure mode in reviews. Say it directly and propose an alternative.
- **Accept override gracefully.** If the author has full context and disagrees, defer. Comment on code, not people.

## Dependency Discipline

Part of code review is dependency review.

**Before adding any Go module dependency:**
1. Does the existing stack solve this? (`net/http`, `database/sql`/pgx, chi, the standard library — often it does.)
2. Is it actively maintained? (Last commit, open issues.)
3. Does it have known vulnerabilities? (`govulncheck ./...`.)
4. What's the license? (Must be compatible — this repo vendors MIT skills, for example.)
5. Will `make build` and `go mod tidy` stay clean?

**Rule:** Prefer the standard library and existing `internal/` utilities over new
dependencies. Every dependency is a liability and attack surface. (Note: `mcp-go`
is still a dependency but the daemon no longer does MCP write-back — write-back is
REST.)

## The Review Checklist

```markdown
## Review: [PR/Change title]

### Context
- [ ] I understand what this change does and which OpenSpec change it implements

### Correctness
- [ ] Change matches the spec delta / task requirements
- [ ] Edge cases handled (nil, empty, zero values, boundaries)
- [ ] Every error checked; wrapped with %w where inspected
- [ ] Job state-machine invariants preserved (allowed transitions, terminal immutable)
- [ ] Tests cover the change; race detector run for concurrency changes

### Readability
- [ ] Names idiomatic and consistent; gofmt-clean
- [ ] Early-return control flow, not deep nesting
- [ ] No unnecessary complexity or premature abstraction

### Architecture
- [ ] Respects internal/ package boundaries and dependency direction
- [ ] New provider (if any) under provider/<name>/ with NO migration
- [ ] Appropriate abstraction level

### Security
- [ ] Prompts to AI CLIs via stdin, never argv
- [ ] pgx queries parameterized; no string concatenation
- [ ] No secrets/tokens/keys in logs or version control
- [ ] Webhook signature verified; rt_token via HMAC-SHA256; correct PAT vs rt_token route guard
- [ ] External data treated as untrusted

### Performance
- [ ] No N+1 against Postgres
- [ ] No unbounded queries; claims use FOR UPDATE SKIP LOCKED
- [ ] No goroutine leaks; blocking calls have context/timeout
- [ ] Pagination on list endpoints

### Verification
- [ ] make vet, make build, make test pass
- [ ] DB-backed tests actually ran (TEST_DATABASE_URL set), not skipped
- [ ] Pipeline changes exercised via internal/integration

### Verdict
- [ ] **Approve** — Ready to merge
- [ ] **Request changes** — Issues must be addressed
```

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "It works, that's good enough" | Working code that's unreadable, insecure, or breaks an invariant creates debt that compounds. |
| "I wrote it, so I know it's correct" | Authors are blind to their own assumptions. Every change benefits from another set of eyes. |
| "We'll clean it up later" | Later never comes. The review is the quality gate — require cleanup before merge. |
| "AI-generated code is probably fine" | AI Go code needs more scrutiny, not less. It's confident and plausible, even when it drops an error or breaks the state machine. |
| "The tests pass, so it's good" | Tests are necessary, not sufficient — and they may have skipped without TEST_DATABASE_URL. They don't catch architecture or readability problems. |

## Red Flags

- PRs merged without any review
- Review that only checks if tests pass (ignoring the other four axes)
- "LGTM" without evidence of actual review
- Security-sensitive changes (auth, tokens, secrets, prompt handling) without a `/security-review` pass
- Large PRs that are "too big to review properly" (split them / split the OpenSpec change)
- No regression test with a bug-fix PR
- A green CI that skipped DB-backed tests
- Review comments without severity labels
- Accepting "I'll fix it later"

## mework notes

- This skill complements the built-in **`/code-review`** and **`/security-review`**
  slash commands and the **`/opsx:address-review`** PR-comment loop. Run those for
  the mechanics; hold the change to this five-axis standard.
- **Lifecycle:** non-trivial work flows through OpenSpec — `/opsx:propose` →
  design/tasks → `/opsx:apply` → review (this skill) → `/opsx:sync` →
  `/opsx:archive`. The autonomous `/opsx:ship` pipeline gates on `make vet` +
  `make test`, so a review that lets a test-skipping or vet-failing change through
  will stall it.
- **Invariants to verify every time** (from CLAUDE.md): prompts to AI CLIs via
  stdin never argv; job state machine transactional with row locks and terminal
  states immutable; webhook de-dup via `UNIQUE(provider_code, external_event_id)`;
  one active job per runtime (partial unique index, `FOR UPDATE SKIP LOCKED`);
  self-retrigger guard; provider-agnostic schema keyed by `(provider_code,
  external_*_id)`; credentials sealed AES-256-GCM at rest and unsealed only
  server-side at write-back (daemon never holds them); `rt_token` lookup via
  HMAC-SHA256; config/credential files `0600`, dirs `0700`.
- **Verification commands:** `make vet`, `make build`, `make test` (`go test -p 1
  ./...`, serialized; DB tests skip without `TEST_DATABASE_URL`; start Postgres
  with `make test-db`). Use the `security-and-hardening` and
  `debugging-and-error-recovery` sibling skills for the security and root-cause
  depth this review references.

## Verification

After review is complete:

- [ ] All Critical issues are resolved
- [ ] All Important issues are resolved or explicitly deferred with justification
- [ ] `make vet`, `make build`, `make test` pass (DB tests actually ran)
- [ ] The CLAUDE.md invariants are intact
- [ ] The verification story is documented (what changed, how it was verified)
