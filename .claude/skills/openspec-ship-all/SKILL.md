---
name: openspec-ship-all
description: Auto-discover every ACTIVE OpenSpec change and ship the full project — apply → ship → archive — locally, automatically, with halt-on-failure and idempotent resume. Use when the user wants `/opsx:ship-all`, "ship everything", "batch ship", "ship the full project", or "run the full pipeline".
license: MIT
compatibility: Requires openspec CLI + ship-plan.js + ship-code.js + openspec-apply-change skill.
metadata:
  author: mework
  version: "1.0"
  generatedBy: "ship-all orchestrator"
---

# Ship All — the per-change decision matrix

This is the source of truth for how the `ship-all` orchestrator decides what to
do with each ACTIVE OpenSpec change. The orchestrator **never** asks per change
— every decision comes from this matrix.

## Per-change decision

For each ACTIVE change, the orchestrator runs `openspec status --change <c> --json`
and `openspec list --json`, then classifies the change by **mode**:

| Mode | Trigger | Steps run by orchestrator |
|---|---|---|
| `apply+ship` | Active, has proposal + design + tasks + specs + `.openspec.yaml`, **0 tasks done** | 1. `/opsx:apply <c>` (per-task implementation loop, ticks tasks) → 2. `Workflow({ name: 'ship-plan', args: { change, date, local: true } })` → 3. `Workflow({ name: 'ship-code', args: { change, date, local: true, bump, noPushMain, archive, mergeStrategy, reserveTokens, maxRepairs } })` |
| `spec+ship` | Active, all artifacts present, **all tasks `[x]`**, no evidence dir | 1. `Workflow({ name: 'spec-change', args: { change, maxRevisions: 1 } })` → 2. `ship-plan` → 3. `ship-code --local` |
| `ship-only` | Active, all tasks `[x]`, evidence dir exists, delta spec already in `openspec/specs/` | 1. `ship-plan` → 2. `ship-code --local` |
| `repair+ship` | Active, **missing `.openspec.yaml`** (scaffolding-only) | 1. `openspec new change <c>` (additive — does NOT touch existing proposal/design/tasks/specs) → 2. re-classify → 3. appropriate ship path |
| `archive-only` | Active, all tasks `[x]`, no `feat/<c>` branch, evidence dir + delta-spec sync complete, no code work expected | 1. `openspec archive <c> -y --skip-specs --no-validate` |
| `skip` | Already ARCHIVED, OR active but no tasks.md (incomplete proposal) | Logged in `.ship-all-progress.json` → `progress.skipped`; never halts the run |

## Cues for the orchestrator agent

1. **Always run `openspec list --json` + per-change `openspec status --change <c> --json` BEFORE deciding modes.** No guessing from the directory listing alone — `cNNNN` filename can be a complete or incomplete proposal.
2. **Sort by cNNNN ordinal** (lexicographic) AFTER expanding `c0014` into `c0014a, c0014b, c0014c`. The numeric ordering respects the dependency graph because each change's proposal was authored in the order the deps required.
3. **Apply `--from <cNNNN>` and `--only <list>` filters** AFTER sorting, so the queue is consistent.
4. **`--dry-run`** runs only Phases 1-2 (Discover + Plan), writes `.ship-all-progress.json`, returns the planned queue. Never commits.
5. **`--skip-apply`** upgrades all `apply+ship` entries to `spec+ship` (treats them as if all tasks were already `[x]`). Useful for resume after partial runs.
6. **`--skip-spec`** downgrades all `spec+ship` entries to `ship-only`. Default in batch mode (the 6-critic spec quality pass is too expensive to run for every change in a batch).
7. **Budget reserve**: the orchestrator passes `reserveTokens` to every nested Workflow call so a budget hit in ship-code halts the entire run cleanly (not mid-pair).

## Halt semantics

The orchestrator **halts on first failure** and returns:

```json
{
  "stage": "apply+ship" | "spec+ship" | "ship-only" | "repair+ship" | "archive-only",
  "ok": false,
  "change": "c0008-object-storage",
  "reason": "...",
  "mergeSha": "...",          // present if merge already happened
  "archivePath": "...",       // present if archive already moved
  "resumeFrom": "c0009-sessions",  // next change to process on resume
  "progressLog": "openspec/changes/.ship-all-progress.json",
  "nextStep": "Fix the failing change locally, then re-run /opsx:ship-all --from <resumeFrom>"
}
```

**Never** rolls back a merged change. The user must `git reset --hard <pre-merge-sha>` themselves if they want to undo.

## Resume protocol

The `.ship-all-progress.json` is the durable state. On re-run:

1. Read the progress file. For each entry with `status=shipped`, **skip** (it's done).
2. For each entry with `status=pending`, **process** (per mode).
3. For each entry with `status=failed`, **skip with warning** (the user must explicitly clear it via `--retry-failed` or fix the change and re-add).

Use `--from <cNNNN>` to start at a specific ordinal (overrides the resume).

## Output

The orchestrator's final report is a per-change summary table + a single
`.ship-all-progress.json` updated atomically (`fs.writeFileSync` with
`JSON.stringify(progress, null, 2)`) after each change so a halt never loses
state.