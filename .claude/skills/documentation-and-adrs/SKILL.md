---
name: documentation-and-adrs
description: Records decisions and documentation, adapted for the mework Go project. Use when making architectural decisions, changing public APIs or the provider adapter contract, shipping features, or recording context future engineers and agents need. Notes how OpenSpec design.md files serve as lightweight ADRs and where docs/ lives.
---

# Documentation and ADRs

## Overview

Document decisions, not just code. The most valuable documentation captures the *why* — the context, constraints, and trade-offs that led to a decision. Code shows *what* was built; documentation explains *why it was built this way* and *what alternatives were considered*. This context is essential for future humans and agents working in the codebase.

## When to Use

- Making a significant architectural decision
- Choosing between competing approaches
- Adding or changing a public API or the provider adapter contract
- Shipping a feature that changes user-facing or webhook-trigger behavior
- Onboarding new team members (or agents) to the project
- When you find yourself explaining the same thing repeatedly

**When NOT to use:** Don't document obvious code. Don't add comments that restate what the code already says. Don't write docs for throwaway prototypes.

## Architecture Decision Records (ADRs)

ADRs capture the reasoning behind significant technical decisions. They're the
highest-value documentation you can write.

**In `mework`, OpenSpec change `design.md` files already serve as lightweight,
per-change ADRs** — they record the problem, the chosen approach, and rejected
alternatives for that change. Reserve standalone ADRs (in `docs/decisions/`) for
**cross-cutting decisions** that span multiple changes/capabilities or that future
proposals must respect: e.g. the provider-agnostic schema, the AES-256-GCM
credential sealing model, prompt-via-stdin, or the pull/poll-vs-SSE direction.

### When to Write a (standalone) ADR

- A decision that constrains *future* OpenSpec changes (not just this one)
- Choosing a major dependency (pgx, chi, goose, goreleaser, mcp-go)
- The provider-agnostic data model and `(provider_code, external_*_id)` identity
- The credential-sealing / secrets strategy (AES-256-GCM, daemon never holds creds)
- The job-queue concurrency model (row locks, `FOR UPDATE SKIP LOCKED`, one active job per runtime)
- The webhook trigger grammar and de-dup invariant
- Any decision that would be expensive to reverse

If a decision lives entirely within one change, capture it in that change's
`design.md` instead of a separate ADR.

### ADR Template

Store standalone ADRs in `docs/decisions/` with sequential numbering:

```markdown
# ADR-001: Provider-agnostic schema keyed by (provider_code, external_*_id)

## Status
Accepted | Superseded by ADR-XXX | Deprecated

## Date
2026-06-20

## Context
mework must connect multiple task platforms (Mello today; Jira / Linear / GitHub
Issues by design). Hard-coding provider columns would force a schema migration
for every new provider and couple the queue to one platform's identifiers.

## Decision
Identify all external entities by (provider_code, external_*_id). Provider-specific
behavior lives behind a Go adapter under internal/server/provider/<name>/, selected
by a registry. Adding a provider adds an adapter, not a migration.

## Alternatives Considered

### Per-provider columns / tables
- Pros: Simple queries for a single provider
- Cons: Every new provider needs a migration; queue logic forks per provider
- Rejected: violates the "add a provider without a migration" invariant

### A single JSON blob per external entity
- Pros: Maximally flexible
- Cons: Loses the UNIQUE(provider_code, external_event_id) de-dup guarantee
- Rejected: webhook de-dup depends on a real unique index

## Consequences
- Webhook de-dup relies on UNIQUE(provider_code, external_event_id)
- New providers ship as adapters under internal/server/provider/
- Spec lives in openspec/specs/provider-gateway/spec.md
```

### ADR Lifecycle

```
PROPOSED → ACCEPTED → (SUPERSEDED or DEPRECATED)
```

- **Don't delete old ADRs.** They capture historical context.
- When a decision changes, write a new ADR that references and supersedes the old one.
- Cross-reference: link the ADR from the relevant `openspec/specs/<capability>/spec.md`
  and from the change `design.md` that introduced or revisited the decision.

## OpenSpec specs vs. ADRs (this repo)

| Artifact | Captures | Lives in |
|---|---|---|
| `openspec/specs/<capability>/spec.md` | The *current* behavior contract (the source of truth) | `openspec/specs/` |
| change `design.md` | The reasoning for *this* change (a lightweight ADR) | `openspec/changes/<name>/design.md` |
| standalone ADR | A cross-cutting decision future changes must respect | `docs/decisions/` |

When behavior changes, update the spec via a change's delta + `/opsx:sync` — don't
let the spec drift from the code.

## Inline Documentation

### When to Comment

Comment the *why*, not the *what*:

```go
// BAD: Restates the code
// increment the counter
counter++

// GOOD: Explains a non-obvious invariant
// Claim under FOR UPDATE SKIP LOCKED so concurrent daemons never grab the same
// job; the partial unique index enforces one active job per runtime, so a second
// claim by the same runtime is skipped rather than blocked.
row := tx.QueryRow(ctx, claimSQL, runtimeID)
```

### When NOT to Comment

```go
// Don't comment self-explanatory code
func total(items []LineItem) int {
    sum := 0
    for _, it := range items {
        sum += it.Price * it.Qty
    }
    return sum
}

// Don't leave TODO comments for things you should just do now
// TODO: add error handling  ← just handle the error

// Don't leave commented-out code  ← delete it, git has history
```

### Document Known Gotchas (this repo has several)

```go
// IMPORTANT: prompts go to the AI CLI over STDIN, never argv. Ticket/comment
// content is attacker-controllable; keeping it off the command line avoids shell
// injection. See the auth-and-secrets spec and CLAUDE.md "Conventions".
cmd.Stdin = strings.NewReader(prompt)
```

Mirror the invariants already listed in `CLAUDE.md` (transactional state machine,
terminal states immutable, self-retrigger guard, file perms 0600/0700) wherever
the code enforces them.

## API Documentation

For public surfaces — the provider adapter interface, the `meworkclient` HTTP
client, and the server's HTTP routes — document with Go doc comments:

```go
// CreateComment posts result back to the provider over its REST API. It is called
// server-side only, after the sealed connection credential is unsealed; the daemon
// never holds provider credentials. Returns the provider's comment ID on success.
//
// Errors are retried via the durable outbox, so CreateComment must be idempotent
// with respect to (provider_code, external_event_id) where the provider allows it.
func (a *Adapter) CreateComment(ctx context.Context, in CommentInput) (string, error) {
    // ...
}
```

For the HTTP routes (`/webhooks/{provider}`, `/api/v1/jobs/*`, management routes),
document the auth model inline (PAT vs. `rt_token` vs. signature-verified) and keep
the canonical description in the matching `openspec/specs/` capability.

## README & docs/ Structure

Project docs live under `docs/`. Key entry points:

| Doc | Purpose |
|---|---|
| `README.md` | Quick start, build/test commands, high-level overview |
| `docs/product-overview.md` | Product description, actors, use cases |
| `docs/openspec-workflow.md` | The spec-driven workflow and artifact formats |
| `docs/target-architecture.md` | The proposed (not-yet-built) agent-hub redesign |
| `docs/codebase-summary.md` | Trusted, current map of the code |
| `CLAUDE.md` | Conventions/invariants for AI agents and humans |

A good README covers quick start, the `make` command table, and an architecture
overview that links to the docs and `openspec/specs/`:

```markdown
## Commands
| Command | Description |
|---------|-------------|
| `make build` | Build bin/mework and bin/mework-server |
| `make vet`   | go vet ./... |
| `make test`  | go test -p 1 ./... (DB tests need TEST_DATABASE_URL) |
| `make test-db` | Start docker Postgres for DB-backed tests |
| `make snapshot` | goreleaser cross-compile (CLI only) |
```

## Changelog Maintenance

The repo keeps a root `CHANGELOG.md` in **Keep a Changelog** format — **one bullet
per shipped change**, generated by `/opsx:ship` when it ships an approved change.
You rarely hand-edit it; keep entries terse and user-facing:

```markdown
# Changelog

## [Unreleased]
### Added
- Webhook trigger grammar routes the `journal` workflow.

### Fixed
- Heartbeat race that could double-claim a runtime.

### Changed
- Job claim now uses FOR UPDATE SKIP LOCKED for fairness under load.
```

## Documentation for Agents

Special consideration for AI agent context:

- **`CLAUDE.md`** — project conventions and invariants; keep it current and accurate.
- **`openspec/specs/`** — the behavior contract agents build against; update via delta + `/opsx:sync`.
- **change `design.md`** — lightweight ADRs that prevent re-deciding settled trade-offs.
- **Standalone ADRs (`docs/decisions/`)** — cross-cutting decisions future changes must respect.
- **Inline gotchas** — prevent agents from falling into the repo's known traps (stdin prompts, terminal-state immutability, no-migration-per-provider).

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "The code is self-documenting" | Code shows what. It doesn't show why, what alternatives were rejected, or which invariant it upholds. |
| "We'll write docs when the API stabilizes" | The spec/design *is* the first test of the design. Write it first via `/opsx:propose`. |
| "Nobody reads docs" | Agents read `CLAUDE.md`, specs, and ADRs. Future engineers do. Your 3-months-later self does. |
| "ADRs are overhead" | A 10-minute design.md or ADR prevents a 2-hour re-litigation of a settled trade-off. |
| "Comments get outdated" | Comments on *why*/invariants are stable. Comments on *what* get outdated — write the former. |
| "I'll just update the code, not the spec" | The spec drifts and the next agent builds the wrong thing. Sync the spec. |

## Red Flags

- Architectural decisions with no written rationale (no design.md, no ADR)
- The code changed behavior but `openspec/specs/` was not updated
- Provider adapter or HTTP routes with no doc comments
- README that doesn't explain how to build/test (`make build` / `make test`)
- Commented-out code instead of deletion
- TODO comments that have lingered for weeks
- A cross-cutting decision (schema, secrets, concurrency) with no ADR
- `CLAUDE.md` that no longer matches the code

## mework notes

- Docs live in `docs/` (e.g. `docs/openspec-workflow.md`, `docs/product-overview.md`,
  `docs/target-architecture.md`, `docs/codebase-summary.md`).
- OpenSpec change `design.md` files already act as lightweight ADRs; reserve
  standalone ADRs in `docs/decisions/` for **cross-cutting** decisions future
  changes must respect.
- The behavior source of truth is `openspec/specs/<capability>/spec.md`; update it
  via a change's delta + `/opsx:sync`, not by editing the spec directly.
- `/opsx:ship` generates the root `CHANGELOG.md` entry (Keep a Changelog, one
  bullet per change) as part of apply → verify → sync → changelog → PR.
- Capture verification evidence under `openspec/changes/<name>/evidence/`.
- See the sibling `git-workflow-and-versioning` skill for commit/PR/CHANGELOG
  mechanics and `ci-cd-and-automation` for the quality gates these decisions ride through.

## Verification

After documenting:

- [ ] Significant decisions have a design.md (per-change) or an ADR (cross-cutting)
- [ ] `openspec/specs/` reflects the new behavior (synced, not drifted)
- [ ] Provider adapter / HTTP route / meworkclient surfaces have Go doc comments
- [ ] Known gotchas (stdin prompts, terminal-state immutability, etc.) are documented inline where they matter
- [ ] README covers `make build` / `make test` and links to docs + specs
- [ ] No commented-out code remains
- [ ] `CLAUDE.md` is current and accurate
