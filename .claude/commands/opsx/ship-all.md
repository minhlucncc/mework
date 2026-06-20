---
name: "OPSX: Ship All"
description: Auto-discover every ACTIVE OpenSpec change and ship the full project — apply → ship → archive — locally, automatically, with halt-on-failure
category: Workflow
tags: [workflow, automation, batch, ship, local, experimental]
---

Ship **every ACTIVE OpenSpec change** through the local pipeline in one go.
The orchestrator auto-decides per change whether to apply+ship, spec+ship,
ship-only, repair+ship, or archive-only — **never asks per change**. Halts on
first failure with full progress. No `gh`, no remote push (unless `--push-main`).

**Input**: Optional `--from <cNNNN>` (start from this ordinal), `--only <list>`
(comma-separated whitelist), `--dry-run` (plan-only), `--skip-apply` (treat all
changes as already-implemented), `--skip-spec` (default true in batch — skip
the 6-critic spec quality pass), `--bump {patch|minor|major}`, `--push-main`,
`--no-archive`, `--merge-strategy {squash|no-ff|ff-only}`, `--force`.

Defaults: squash merge, noPushMain=true, archive=true, skipSpec=true (faster),
skipApply=false. Per-change evidence is always written.

**Steps**

1. **One confirmation gate.** The slash command does NOT immediately launch
   the workflow — instead, it always runs a dry-run first to discover the
   queue, then asks the user to confirm.

   a. Launch a dry-run via the Workflow tool:
      ```
      Workflow({ name: 'ship-all', args: { dryRun: true, ...other-args } })
      ```
      Capture the returned `queue`, `skipped`, and `stats`.

   b. Show the queue as a table (change | mode | tasks | status | reason).

   c. AskUserQuestion with options:
      - **Ship them** — proceed with the planned queue
      - **Edit the queue first** — point at `openspec/changes/.ship-all-progress.json`,
        stop (user edits and re-runs)
      - **Cancel**

2. **Launch the real workflow.** On "Ship them":
   ```
   Workflow({ name: 'ship-all', args: {
     date, fromChange, onlyChange, skipApply, skipSpec,
     mergeStrategy, bump, noPushMain, archive, reserveTokens, maxRepairs, force
   }})
   ```

3. **Surface progress.** As each change ships, show the result: change name,
   mode, per-change duration, evidence dir, mergeSha, archivePath, tag. On
   halt, show the failing change + reason + last commit + suggested fix.

4. **Relay the final report.** Per-change summary table (change | mode |
   commits | mergeSha | archivePath | tag | durationMs) + total stats +
   resume instructions. Always includes the `resumeFrom` change so a halted
   run can be picked up with `--from`.

**Guardrails**
- Halt on first failure. **Never** roll back merged changes. The merge is local;
  if the user wants to undo it: `git reset --hard <pre-merge-sha>`.
- Per-change evidence is always written, even on failure.
- `--dry-run` is the recommended first invocation — it surfaces the queue
  without committing anything.
- The orchestrator refuses to start unless the tree is clean OR `--force`.
- Skill `.claude/skills/openspec-ship-all/SKILL.md` is the source of truth
  for the per-change decision matrix.
- Resume: re-running with `--from <cNNNN>` picks up where the previous run
  stopped. Already-shipped entries are skipped (status check via the per-change
  `openspec status` — entries with `feat/<c>` deleted and `archive/<c>` present
  are considered done).