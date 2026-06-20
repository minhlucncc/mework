---
name: spec-review-and-quality
description: Reviews OpenSpec proposals and delta specs for quality (mework-adapted). Use when writing or reviewing a spec, proposal, requirements, or scenarios — before /opsx:apply or /opsx:ship. Use when you need to judge whether a spec is clean, minimal, testable, complete, and consistent with the project's KISS/YAGNI/DRY philosophy and the CLAUDE.md invariants. Powers the /opsx:spec authoring workflow; the spec-layer counterpart of code-review-and-quality.
---

# Spec Review and Quality

## Overview

Multi-axis review of OpenSpec **specs** (proposals + delta specs), the way
`code-review-and-quality` reviews Go code. A spec is the source the tests and the
implementation are derived from — `/opsx:ship` writes its failing tests from the
spec's acceptance scenarios — so a vague, bloated, or partial spec produces a
vague, bloated, or **partial feature**. Review every spec across six axes before
it drives code.

This skill is the standard behind the **`/opsx:spec`** workflow
(`.claude/workflows/spec-change.js`): its critic agents each take one axis below,
and its revise loop fixes Blocker/Required findings until the spec is clean. Use
`/opsx:spec` for the mechanics; use this skill for the bar you hold the spec to.

**The approval standard:** a spec is ready when every requirement is a single,
testable behavior that the system actually needs now, stated once, in the
project's vocabulary, and fully covered by scenarios and tasks. Approve when it
meets that bar — not when it is maximal. More requirements is not better; the
right requirements, minimally stated, is better.

It complements — does not replace — `openspec validate`. Validation is the
**mechanical gate** (structure/schema). This skill is the **judgment gate**
(clarity, minimality, testability, completeness).

## When to Use

- Right after `/opsx:propose` drafts a change, before `/opsx:apply` or `/opsx:ship`.
- When requirements or scenarios were written by another agent or a human.
- When a spec "passes `openspec validate`" but you're unsure it's actually good.
- When a change keeps getting re-touched (a sign a requirement is doing too much).

## The Six-Axis Review

### 1. Structure & validity *(mechanical — Blocker if it fails)*

The schema floor. Enforced by `openspec validate --strict`, re-checked here:

- Main spec: a `## Purpose` (one paragraph) + a `## Requirements` section.
- Every `### Requirement:` has `SHALL`/`MUST` **in the body line** (not only the
  header), and **at least one** `#### Scenario:` with `- **WHEN**` / `- **THEN**`.
- Delta specs use only `## ADDED/MODIFIED/REMOVED/RENAMED Requirements`; a
  MODIFIED requirement includes the **full** updated requirement (header + body +
  all scenarios), not just the delta line.
- A MODIFIED/REMOVED/RENAMED requirement name must exist in the baseline
  `openspec/specs/<capability>/spec.md`.

### 2. Clarity & KISS

One requirement = one behavior.

- If a requirement body needs "and … and …" across distinct concerns, **split it**.
  (Smell: webhook "Trigger grammar" bundling token-order + recognized-set +
  word-boundary + case-normalization; job-queue "Heartbeat and lease" bundling
  heartbeat + the running/done/failed acks.)
- The body is a plain sentence a reader understands without the design doc.
- Requirement names are specific noun phrases ("Idempotent enqueue"), not vague
  ("Handle webhooks well").

### 3. Testability

Every scenario must be decidable by a test.

- `WHEN`/`THEN` assert a **definite, observable** outcome with **concrete
  literals** (`@mework dev review fix the login bug`, `code-fixer@1.2.0`), not a
  paraphrase of the requirement.
- **No soft modifiers in the THEN of an intended behavior** — `MAY`,
  "to the extent that…", "if possible". A tester can't assert those. If a behavior
  is genuinely optional, move it into its own requirement explicitly marked
  optional, and give the *guaranteed* part a hard THEN.
- A negative/abuse scenario exists where it matters (the `test@mework.com`
  not-a-trigger case is the model).
- Could you write the Go table-test row from this scenario without inventing
  details? If not, it's underspecified.

### 4. Minimality & YAGNI

Spec what's built and needed now — nothing else.

- Normative `### Requirement:` SHALLs describe **in-scope** behavior only. Future
  options (extra broker backends like NATS/Redis, a multi-module layout, an
  artifact form nothing consumes yet) belong in `design.md` or a clearly
  non-normative note — **not** as requirements. (The PRD's own YAGNI rule: *"Không
  phát triển quá mức … khi chưa cần thiết."*)
- **Spec behavior, not process.** "The existing test suite MUST pass",
  "gofmt-clean", "land in one PR" are tasks/process, not system requirements.
- No requirement that adds no observable behavior. If removing it changes no test,
  delete it.

### 5. Consistency & DRY

One source of truth, one vocabulary.

- A behavior is defined in **exactly one** capability spec; other specs
  **reference** it ("see the `message-bus` capability") rather than restating it.
  Shared contracts (wire format, IDs) are specced once.
- **Canonical terminology** (see Glossary): use `runner`, `dispatch`, `hub`,
  `session`, `grant`, `sandbox`, `agent`. Map legacy baseline terms when you touch
  them. Don't mix `runtime`/`runner` or `job`/`dispatch` within one redesign spec.
- Consistent with the **CLAUDE.md invariants** (stdin-not-argv; transactional
  state machine with immutable terminals; de-dup on `(provider_code,
  external_event_id)`; provider-agnostic schema; 0600/0700; sealed credentials). A
  requirement that contradicts an invariant is a **Blocker**.

### 6. Completeness — "ship complete features, not partials" *(cross-artifact)*

The spec must fully define the feature so `/opsx:ship` can finish it.

- **proposal → specs:** every capability/behavior the proposal claims has a
  matching requirement. No "What Changes" bullet without a home.
- **requirement → scenarios:** each requirement has ≥1 scenario covering the happy
  path **and** the meaningful edge/failure (dedup, expiry, denial, reconnect).
- **requirement → tasks:** each requirement is covered by task(s) in `tasks.md`,
  and each task traces to a requirement. A requirement with no task ships as a
  partial; a task with no requirement is unspecced work.
- **design coverage:** every non-trivial decision the proposal implies is recorded
  in `design.md` (so it isn't re-litigated mid-implementation).

## Severity Labels

Label every finding so the author knows what's required vs optional (mirrors
`code-review-and-quality`):

| Prefix | Meaning | Action |
|--------|---------|--------|
| **Blocker:** | Invalid, untestable, or contradicts a CLAUDE.md invariant | Must fix; revise loop won't pass |
| *(no prefix)* | Required | Fix before `/opsx:apply` |
| **Nit:** | Minor/style (wording, ordering) | Optional |
| **FYI** | Informational / future context | No action |

The `/opsx:spec` revise loop exits only when **no Blocker and no Required** finding
remains (or `maxRevisions` is hit — residual items are reported, never hidden).

## Spec Sizing

A change should map to a reviewable diff. If its requirements imply a ~1000-line
diff, the change is too big — **split the proposal** (by capability, or
horizontal: shared contract/migration first, then consumers). One change = one
coherent capability slice, not a whole subsystem. (See `code-review-and-quality`
→ Change Sizing; the same splitting strategies apply at the spec level.)

## Review Process

1. **Understand intent.** Read `proposal.md` (what & why) and `design.md` (how)
   before judging requirements.
2. **Validate first (Axis 1).** Run `openspec validate "<change>" --strict`. A
   structural failure is a Blocker — fix before judging the rest.
3. **Review each requirement against Axes 2–5.** One behavior? Testable scenarios?
   Minimal/YAGNI? Consistent vocabulary and invariant-safe?
4. **Cross-check coverage (Axis 6).** proposal↔specs↔tasks↔design — build the trace
   and flag every gap.
5. **Categorize** every finding with a severity label.

## Multi-Agent Review Pattern

The `/opsx:spec` workflow fans out **one critic agent per axis in parallel** —
independent reviewers have different blind spots, and a spec critique is read-only
so parallel review is safe (unlike code fixes that touch shared files). Each
critic returns `{axis, severity, location, problem, suggestion}`; a reviser agent
then applies Blocker/Required fixes sequentially and re-validates. Author writes →
critics review → reviser fixes → re-validate → human approves.

## Glossary (canonical terms)

The redesign vocabulary is canonical going forward; baseline specs keep their
terms until superseded, but **new and revised specs use the canonical column**.

| Canonical | Legacy / avoid | Meaning |
|-----------|----------------|---------|
| **hub** | (server, when acting as broker) | the central server: registry, broker, catalog, orchestrator, sessions |
| **runner** | runtime, daemon, client | the enrolled local worker that subscribes, pulls, runs, reports |
| **dispatch** | job, claim | a unit of work the hub publishes to a runner/session topic |
| **session** | — | a live runner↔hub association over the SSE channel |
| **grant** | — | a scoped, least-privilege permission set carried by a dispatch |
| **sandbox** | (workdir) | the isolated runtime that runs one agent |
| **agent** | profile (static) | a versioned, pullable unit of instruction/behavior |

Within a single spec, never mix a canonical term and its legacy synonym for the
same concept. When a delta touches a baseline spec that still uses a legacy term,
keep that spec's term unless the change's purpose is the rename (use `## RENAMED
Requirements`).

## The Review Checklist

```markdown
## Spec Review: [change name]

### Axis 1 — Structure & validity
- [ ] openspec validate --strict passes
- [ ] Purpose + Requirements present; SHALL/MUST in body; ≥1 scenario each
- [ ] Delta uses ADDED/MODIFIED/REMOVED/RENAMED; MODIFIED includes full requirement
- [ ] MODIFIED/REMOVED names exist in the baseline spec

### Axis 2 — Clarity & KISS
- [ ] One requirement = one behavior (no "and…and…" packing)
- [ ] Plain-language bodies; specific requirement names

### Axis 3 — Testability
- [ ] Scenarios use concrete literals and definite WHEN/THEN
- [ ] No soft MAY / "to the extent of" in an intended-behavior THEN
- [ ] Edge/negative scenarios where they matter

### Axis 4 — Minimality & YAGNI
- [ ] No requirement specs an unbuilt/future option (those live in design.md)
- [ ] Spec behavior, not process/test mechanics
- [ ] No requirement that changes no test

### Axis 5 — Consistency & DRY
- [ ] Each behavior defined once; cross-references instead of restating
- [ ] Canonical glossary terms; no mixed vocabulary
- [ ] Consistent with the CLAUDE.md invariants (no contradiction)

### Axis 6 — Completeness (not partials)
- [ ] Every proposal claim → a requirement
- [ ] Every requirement → ≥1 scenario AND covering task(s) in tasks.md
- [ ] Every task → a requirement; design.md records the non-trivial decisions

### Verdict
- [ ] Approve — clean, minimal, testable, complete
- [ ] Revise — Blocker/Required findings remain
```

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "It passes `openspec validate`" | Validation is structural. A valid spec can still be vague, bloated, or partial. |
| "More requirements = more thorough" | Bloat. YAGNI: spec what's needed now; future options go in design.md. |
| "The scenario says it MAY happen" | A `MAY` in a behavior's THEN is untestable. Make the guaranteed part hard; isolate true optionality. |
| "We'll add the missing scenario later" | Later the test author guesses. Cover edge/failure cases in the spec now. |
| "Both 'runner' and 'runtime' are clear enough" | Mixed vocabulary compounds across specs. Pick the canonical term. |

## Red Flags

- A requirement body with multiple SHALLs across unrelated concerns.
- A scenario whose THEN restates the requirement instead of asserting an outcome.
- Enumerated future backends/forms/layouts in a normative requirement.
- A "What Changes" proposal bullet with no matching requirement, or a requirement
  with no task — the partial-feature smell.
- A change that re-touches the same requirement repeatedly (it's doing too much).
- A requirement that quietly contradicts a CLAUDE.md invariant.

## mework notes

- **Lifecycle:** `/opsx:explore` → `/opsx:propose` (draft) → **`/opsx:spec` (review +
  revise — this skill)** → `/opsx:apply` or `/opsx:ship` → `/opsx:sync` →
  `/opsx:archive`. `/opsx:propose` is the fast single-pass primitive; `/opsx:spec`
  is the quality-assured path.
- **Counterpart skills:** `spec-driven-development` (what to put in the artifacts +
  the six coverage areas), `planning-and-task-breakdown` (design.md/tasks.md
  shape), `code-review-and-quality` (the same standard, one layer down on code).
- **Verification:** `openspec validate "<change>" --strict --no-interactive` for
  the mechanical floor; this skill's six axes for the judgment above it.
