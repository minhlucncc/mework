---
name: spec-driven-development
description: Creates specs before coding (mework-adapted onto OpenSpec). Use when starting a new feature or significant change and no OpenSpec change exists yet. Use when requirements are unclear, ambiguous, or only exist as a vague idea. In mework this maps SPECIFY/PLAN/TASKS/IMPLEMENT onto /opsx:propose, design.md, tasks.md, and /opsx:apply — do not invent a competing PRD file.
---

# Spec-Driven Development

## Overview

Write a structured specification before writing any code. The spec is the shared source of truth between you and the human engineer — it defines what we're building, why, and how we'll know it's done. Code without a spec is guessing.

**`mework` already implements spec-driven development via [OpenSpec](https://github.com/Fission-AI/OpenSpec).** Don't author a freeform PRD file; the artifacts are generated and tracked by the `/opsx:*` workflow under `openspec/changes/<name>/`. This skill explains *what to put in those artifacts* — it doesn't replace them.

## When to Use

- Starting a new feature or capability
- Requirements are ambiguous or incomplete
- The change touches multiple files or modules
- You're about to make an architectural decision
- The task would take more than 30 minutes to implement

**When NOT to use:** Single-line fixes, typo corrections, or changes where requirements are unambiguous and self-contained.

## The Gated Workflow (mapped onto OpenSpec)

Spec-driven development has four phases. Do not advance to the next phase until the current one is validated by a human. Each phase corresponds to an OpenSpec step:

```
SPECIFY ────────→ PLAN ────────→ TASKS ────────→ IMPLEMENT
/opsx:propose     design.md      tasks.md        /opsx:apply
(proposal.md +    (decisions &   (ordered,       (or /opsx:ship
 delta specs)      rationale)     verifiable)      when approved)
   │                 │              │                 │
   ▼                 ▼              ▼                 ▼
 Human            Human          Human             Human
 reviews          reviews        reviews           reviews
```

`/opsx:explore` is the optional pre-SPECIFY think-only mode for shaping a vague idea before you propose.

### Phase 1: Specify → `/opsx:propose`, then quality-assure → `/opsx:spec`

Start with a high-level vision. Ask the human clarifying questions until requirements are concrete, then run `/opsx:propose "<name>"`, which generates `proposal.md` and the delta specs under `openspec/changes/<name>/specs/`.

`/opsx:propose` is a **single-pass draft** — it generates artifacts but does not judge their quality. Before planning or implementing, run **`/opsx:spec <name>`** to cross-validate the draft across the six spec-review axes (Structure/validity · Clarity/KISS · Testability · Minimality/YAGNI · Consistency/DRY · Completeness) and revise it until clean. The quality bar is the `spec-review-and-quality` skill; the six coverage areas below are *what* to cover, those axes are *how well*. A clean spec is what lets `/opsx:ship` produce a complete feature, not a partial one.

**Surface assumptions immediately.** Before writing proposal content, list what you're assuming, in `proposal.md`:

```
ASSUMPTIONS I'M MAKING:
1. This change affects the webhook-pipeline capability (not job-queue)
2. The new provider reuses the (provider_code, external_*_id) schema — no migration
3. Write-back stays REST via the durable outbox (not MCP)
4. Credentials are sealed with AES-256-GCM like every other connection
→ Correct me now or I'll proceed with these.
```

Don't silently fill in ambiguous requirements. The proposal's entire purpose is to surface misunderstandings *before* code gets written — assumptions are the most dangerous form of misunderstanding.

**The proposal + delta specs must cover these six core areas** (use this as a review checklist):

1. **Objective** — What are we building and why? Who is the actor (developer, server, daemon)? What does success look like? Which `openspec/specs/<capability>` baseline does it extend or change?

2. **Commands** — Full executable commands with flags, not just tool names.
   ```
   Build: make build            # or: go build ./...
   Vet:   make vet              # go vet ./...
   Test:  make test             # go test -p 1 ./...  (serialized; shares one Postgres DB)
   DB:    make test-db          # docker Postgres; export TEST_DATABASE_URL to enable DB tests
   ```

3. **Project Structure** — Where the code lives. Name the concrete packages the change touches, e.g.:
   ```
   cmd/mework/                 → CLI + daemon entrypoint (cmd_*.go)
   cmd/mework-server/          → provider-gateway server entrypoint
   internal/server/webhook/    → /webhooks/{provider}, ParseTrigger, enqueue
   internal/server/jobs/       → job lifecycle, state machine, sweeper, write-back
   internal/server/provider/   → provider adapters (add new ones under <name>/)
   internal/agentrun/          → AI CLI detection + execution (prompt via stdin)
   internal/integration/       → end-to-end pipeline test (TestFullPipelineE2E)
   openspec/changes/<name>/    → this change's proposal, specs, design, tasks, evidence
   ```

4. **Code Style** — One real Go snippet showing the convention beats three paragraphs describing it. Match the repo: errors wrapped with `fmt.Errorf("...: %w", err)`, table-driven tests, `net/http/httptest` for HTTP handlers.
   ```go
   func (s *Store) Enqueue(ctx context.Context, j Job) (string, error) {
       id, err := s.insert(ctx, j)
       if err != nil {
           return "", fmt.Errorf("enqueue job %s/%s: %w", j.ProviderCode, j.ExternalEventID, err)
       }
       return id, nil
   }
   ```

5. **Testing Strategy** — `go test -p 1 ./...` (serialized — DB-backed tests share one Postgres DB and skip unless `TEST_DATABASE_URL` is set). State which level fits: table-driven unit tests, HTTP handler tests with `net/http/httptest`, or the `internal/integration` end-to-end pipeline test for cross-cutting flows.

6. **Boundaries** — Three-tier system:
   - **Always do:** Run `make vet` + `make test` before commits; keep prompts on stdin; identify entities by `(provider_code, external_*_id)`; seal credentials with AES-256-GCM; `0600` files / `0700` dirs.
   - **Ask first:** New goose migration, new dependency, new provider adapter, changes to the job state machine, CI config changes.
   - **Never do:** Commit secrets; pass ticket content on argv; mutate a terminal job state; require a migration to add a provider.

**Do not write a separate spec template file.** `/opsx:propose` produces `proposal.md` and delta specs in the OpenSpec scenario format. Fill those; the six areas above are your coverage checklist for them.

**Reframe instructions as success criteria.** When receiving vague requirements, translate them into concrete, testable conditions in `proposal.md`:

```
REQUIREMENT: "Make webhook handling more reliable"

REFRAMED SUCCESS CRITERIA:
- Duplicate deliveries de-dup on UNIQUE(provider_code, external_event_id)
- A claimed-but-dead runtime's job returns to queued within one sweeper cycle
- TestFullPipelineE2E passes with a simulated redelivery
→ Are these the right targets?
```

### Phase 2: Plan → `design.md`

With the validated proposal, capture the technical plan in the change's `design.md`:

1. Identify the major components and their dependencies (which `internal/...` packages, in what order)
2. Determine the implementation order (foundations first — e.g. migration/schema before handler)
3. Note risks and mitigation strategies (state-machine edge cases, dedup races)
4. Identify what can be built in parallel vs. sequential
5. Define verification checkpoints between phases

`design.md` should be reviewable: the human reads it and says "yes, that's the right approach" or "no, change X." See `planning-and-task-breakdown` for the design and dependency-graph mechanics.

### Phase 3: Tasks → `tasks.md`

Break the plan into discrete, implementable tasks in `tasks.md` (generated by `/opsx:propose`, refined as needed):

- Each task is completable in a single focused session
- Each task has explicit acceptance criteria
- Each task includes a verification step (`make test`, `make vet`, manual check)
- Tasks are ordered by dependency, not perceived importance
- No task should require changing more than ~5 files

`tasks.md` uses OpenSpec checkboxes; tick them off during `/opsx:apply`. The task structure and sizing rules live in `planning-and-task-breakdown` — follow that skill rather than duplicating it here.

### Phase 4: Implement → `/opsx:apply` (or `/opsx:ship`)

Execute tasks one at a time via `/opsx:apply`, ticking `tasks.md` as you go. Follow `incremental-implementation` (thin vertical slices) and `test-driven-development` (failing test first). When the change's spec is human-**approved**, `/opsx:ship` runs the whole pipeline autonomously: apply → verify (`make vet`/`make test` + `openspec validate`) → sync → changelog → commit → push → open PR.

## Keeping the Spec Alive

The spec is a living document, not a one-time artifact:

- **Update when decisions change** — If the data model needs to change, update the change's delta specs / `design.md` first, then implement.
- **Update when scope changes** — Features added or cut should be reflected in `proposal.md` and `tasks.md`.
- **Commit the change** — The whole `openspec/changes/<name>/` tree belongs in version control alongside the code.
- **Sync and archive** — Run `/opsx:sync` to merge delta specs into `openspec/specs/`, and `/opsx:archive` after the PR merges so the baseline always reflects reality.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "This is simple, I don't need a proposal" | Simple tasks don't need *long* proposals, but they still need acceptance criteria. A short proposal is fine. |
| "I'll write the spec after I code it" | That's documentation, not specification. The proposal's value is forcing clarity *before* code. |
| "The spec will slow us down" | A 15-minute proposal prevents hours of rework. |
| "Requirements will change anyway" | That's why OpenSpec changes are living documents. An outdated proposal still beats none. |
| "I'll just write my own PRD" | Don't — it competes with OpenSpec. Use `/opsx:propose`. |

## Red Flags

- Starting to write code without an OpenSpec change
- Asking "should I just start building?" before clarifying what "done" means
- Implementing features not mentioned in `proposal.md` or `tasks.md`
- Making architectural decisions without recording them in `design.md`
- Hand-writing a PRD file that duplicates or conflicts with OpenSpec

## Verification

Before proceeding to implementation, confirm:

- [ ] The proposal + delta specs cover all six core areas (Objective, Commands, Project Structure, Code Style, Testing Strategy, Boundaries)
- [ ] `/opsx:spec` ran and the spec review verdict is **approve** (no Blocker/Required findings across the six axes)
- [ ] The human has reviewed and approved the change
- [ ] Success criteria are specific and testable
- [ ] Boundaries (Always/Ask First/Never) are defined
- [ ] `design.md` and `tasks.md` exist and `openspec validate` passes

## mework notes

- This skill is the SPECIFY/PLAN/TASKS/IMPLEMENT mapping onto OpenSpec: SPECIFY =
  `/opsx:propose` (proposal.md + delta specs), PLAN = `design.md`, TASKS =
  `tasks.md`, IMPLEMENT = `/opsx:apply` (or `/opsx:ship` once approved). Never
  introduce a parallel PRD format.
- Read the relevant baseline at `openspec/specs/<capability>/spec.md` before
  proposing — it is the canonical record of already-implemented behavior.
- Bake the repo invariants into Boundaries: prompts to AI CLIs over **stdin, never
  argv**; transactional job state machine with immutable terminal states;
  provider-agnostic schema keyed by `(provider_code, external_*_id)` (no migration
  to add a provider); AES-256-GCM-sealed credentials; `0600` files / `0700` dirs.
- Capture verification evidence under `openspec/changes/<name>/evidence/`
  (`make vet` / `make test` output). DB-backed tests skip unless `TEST_DATABASE_URL`
  is set; `make test` runs `go test -p 1 ./...` serialized over one Postgres DB.
