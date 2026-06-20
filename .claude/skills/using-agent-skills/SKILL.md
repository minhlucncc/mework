---
name: using-agent-skills
description: Discovers and invokes agent skills (mework-adapted). Use when starting a session or when you need to discover which skill applies to the current task. This is the meta-skill that governs how all other skills are discovered and invoked, and it routes work onto mework's OpenSpec lifecycle (/opsx:propose → design → tasks → /opsx:apply → verify → /opsx:ship).
---

# Using Agent Skills

## Overview

Agent Skills is a collection of engineering workflow skills organized by development phase. Each skill encodes a specific process that senior engineers follow. This meta-skill helps you discover and apply the right skill for your current task.

In `mework`, the lifecycle is anchored to **OpenSpec**: non-trivial work starts as a change proposal, not code. The routing table below maps each development phase to the matching skill(s) *and* the `/opsx:*` command that drives it.

## Skill Discovery

When a task arrives, identify the development phase and apply the corresponding skill. The `/opsx:*` column shows where the OpenSpec workflow plugs in.

```
Task arrives
    │
    ├── Don't know what you want yet? ──────→ /opsx:explore  (think-only mode, no code)
    ├── New project/feature/change? ────────→ /opsx:propose  + spec-driven-development
    │                                          (writes proposal.md + delta specs)
    ├── Reviewing/revising a spec? ──────────→ /opsx:spec  + spec-review-and-quality
    │                                          (6-axis cross-validate → revise until clean)
    ├── Have a proposal, need a design? ─────→ design.md      + planning-and-task-breakdown
    │                                          (decisions, dependency graph, tasks.md)
    ├── Writing/running tests first (Red)? ──→ test-driven-development
    ├── Implementing code (Green)? ──────────→ incremental-implementation + /opsx:apply
    │                                          (tick tasks.md as you go)
    ├── Something broke? ────────────────────→ debugging-and-error-recovery
    ├── Reviewing code? ─────────────────────→ code-review-and-quality
    │   ├── Security concerns? ──────────────→ security-and-hardening
    │   └── Too complex? ────────────────────→ code-simplification
    ├── Committing/branching/versioning? ────→ git-workflow-and-versioning
    ├── CI/CD pipeline work? ────────────────→ ci-cd-and-automation
    ├── Writing docs/ADRs? ──────────────────→ documentation-and-adrs
    ├── Shipping an approved change? ────────→ /opsx:ship  (apply → verify → sync →
    │                                          changelog → PR) then /opsx:archive post-merge
    └── Responding to PR review feedback? ───→ /opsx:address-review
```

## Core Operating Behaviors

These behaviors apply at all times, across all skills. They are non-negotiable.

### 1. Surface Assumptions

Before implementing anything non-trivial, explicitly state your assumptions:

```
ASSUMPTIONS I'M MAKING:
1. [assumption about requirements]
2. [assumption about architecture]
3. [assumption about scope]
→ Correct me now or I'll proceed with these.
```

Don't silently fill in ambiguous requirements. The most common failure mode is making wrong assumptions and running with them unchecked. Surface uncertainty early — it's cheaper than rework. In mework, capture these in the change's `proposal.md` so the human reviews them before `/opsx:apply`.

### 2. Manage Confusion Actively

When you encounter inconsistencies, conflicting requirements, or unclear specifications:

1. **STOP.** Do not proceed with a guess.
2. Name the specific confusion.
3. Present the tradeoff or ask the clarifying question.
4. Wait for resolution before continuing.

**Bad:** Silently picking one interpretation and hoping it's right.
**Good:** "I see X in the `openspec/specs/` baseline but Y in the existing code. Which takes precedence?"

### 3. Push Back When Warranted

You are not a yes-machine. When an approach has clear problems:

- Point out the issue directly
- Explain the concrete downside (quantify when possible — "this adds ~200ms latency" not "this might be slower")
- Propose an alternative
- Accept the human's decision if they override with full information

Sycophancy is a failure mode. "Of course!" followed by implementing a bad idea helps no one. Honest technical disagreement is more valuable than false agreement.

### 4. Enforce Simplicity

Your natural tendency is to overcomplicate. Actively resist it.

Before finishing any implementation, ask:
- Can this be done in fewer lines?
- Are these abstractions earning their complexity?
- Would a staff engineer look at this and say "why didn't you just..."?

If you build 1000 lines and 100 would suffice, you have failed. Prefer the boring, obvious solution. Cleverness is expensive.

### 5. Maintain Scope Discipline

Touch only what you're asked to touch.

Do NOT:
- Remove comments you don't understand
- "Clean up" code orthogonal to the task
- Refactor adjacent systems as a side effect
- Delete code that seems unused without explicit approval
- Add features not in the change's `proposal.md`/`tasks.md` because they "seem useful"

Your job is surgical precision, not unsolicited renovation.

### 6. Verify, Don't Assume (Evidence Required)

Every skill includes a verification step. A task is not complete until verification passes. "Seems right" is never sufficient — there must be evidence (`make vet`, `make test`, build output, runtime data).

Capture that evidence under `openspec/changes/<name>/evidence/` so reviewers can see proof — command output, test logs, screenshots — not just claims.

## Failure Modes to Avoid

These are the subtle errors that look like productivity but create problems:

1. Making wrong assumptions without checking
2. Not managing your own confusion — plowing ahead when lost
3. Not surfacing inconsistencies you notice
4. Not presenting tradeoffs on non-obvious decisions
5. Being sycophantic ("Of course!") to approaches with clear problems
6. Overcomplicating code and APIs
7. Modifying code or comments orthogonal to the task
8. Removing things you don't fully understand
9. Building without a proposal because "it's obvious"
10. Skipping verification (or not saving evidence) because "it looks right"

## Skill Rules

1. **Check for an applicable skill before starting work.** Skills encode processes that prevent common mistakes.

2. **Skills are workflows, not suggestions.** Follow the steps in order. Don't skip verification steps.

3. **Multiple skills can apply.** A feature might involve `spec-driven-development` (`/opsx:propose`) → `planning-and-task-breakdown` (design.md + tasks.md) → `test-driven-development` → `incremental-implementation` (`/opsx:apply`) → `code-review-and-quality` → `code-simplification` → `git-workflow-and-versioning` (`/opsx:ship`) in sequence.

4. **When in doubt, start with a proposal.** If the task is non-trivial and there's no OpenSpec change, begin with `spec-driven-development` and run `/opsx:propose`.

## Lifecycle Sequence (mework / OpenSpec)

For a complete change, the typical sequence is:

```
1.  /opsx:explore               → Think through the idea (no code)
2.  spec-driven-development     → /opsx:propose: proposal.md + delta specs
2b. spec-review-and-quality     → /opsx:spec: cross-validate 6 axes → revise until clean
3.  planning-and-task-breakdown → design.md (decisions) + tasks.md (ordered tasks)
4.  test-driven-development     → Red: failing test first for each slice
5.  incremental-implementation  → Green: /opsx:apply, build slice by slice, tick tasks.md
6.  debugging-and-error-recovery→ Reproduce → localize → fix → guard when something breaks
7.  code-review-and-quality     → Review before merge
8.  security-and-hardening      → Validate inputs, least privilege, secrets handling
9.  code-simplification         → Reduce complexity while preserving behavior
10. git-workflow-and-versioning → Clean commits / branch
11. documentation-and-adrs      → Document the why
12. /opsx:ship                  → apply → verify (make vet/test) → sync → changelog → PR
13. /opsx:address-review        → Respond to PR feedback
14. /opsx:archive               → After PR merges, move change to openspec/changes/archive/
```

Not every task needs every skill. A bug fix might only need: `debugging-and-error-recovery` → `test-driven-development` → `code-review-and-quality`.

## Quick Reference

| Phase | Skill / Command | One-Line Summary |
|-------|-----------------|------------------|
| Explore | `/opsx:explore` | Think through an idea, clarify requirements — no code written |
| Define | spec-driven-development + `/opsx:propose` | Proposal and delta specs before code |
| Review spec | spec-review-and-quality + `/opsx:spec` | 6-axis cross-validate → revise until clean, minimal, testable, complete |
| Plan | planning-and-task-breakdown (design.md + tasks.md) | Decompose into small, verifiable tasks |
| Verify (Red) | test-driven-development | Failing test first, then make it pass |
| Build (Green) | incremental-implementation + `/opsx:apply` | Thin vertical slices, tick tasks.md, verify each |
| Verify | debugging-and-error-recovery | Reproduce → localize → fix → guard |
| Review | code-review-and-quality | Five-axis review with quality gates |
| Review | security-and-hardening | Input validation, least privilege, AES-256-GCM secrets |
| Review | code-simplification | Preserve behavior while reducing complexity |
| Ship | git-workflow-and-versioning | Atomic commits, clean history |
| Ship | ci-cd-and-automation | Automated quality gates (`make vet`, `make test`) |
| Ship | documentation-and-adrs | Document the why, not just the what |
| Ship | `/opsx:ship` | Autonomous apply → verify → sync → changelog → PR |
| Ship | `/opsx:address-review` | Respond to PR review feedback |
| Ship | `/opsx:archive` | Finalize change after PR merges |

## mework notes

- The router's "spec" node is OpenSpec: never hand-write a competing PRD format —
  run `/opsx:propose` and let it generate `proposal.md`, delta specs, `design.md`,
  and `tasks.md`. Read the relevant `openspec/specs/<capability>/spec.md` baseline
  (`provider-gateway`, `webhook-pipeline`, `job-queue`, `rest-writeback`,
  `daemon-runtime`, `cli`, `auth-and-secrets`) before changing a subsystem.
- **Evidence lives at `openspec/changes/<name>/evidence/`.** Rule 6 (verify, don't
  assume) is satisfied by `make vet` + `make test` (`go test -p 1 ./...`) output,
  not by assertion. DB-backed tests skip unless `TEST_DATABASE_URL` is set.
- Honor the repo invariants while routing any task: prompts go to AI CLIs over
  **stdin, never argv**; the job state machine is transactional with row locks and
  terminal states are immutable; the schema is provider-agnostic, keyed by
  `(provider_code, external_*_id)`; credentials are sealed with AES-256-GCM;
  config/credential files are `0600`, dirs `0700`.
- The `/opsx:ship` pipeline is the autonomous version of the Ship phase and only
  runs against a change whose spec is already **approved** by a human.
