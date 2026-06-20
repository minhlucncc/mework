---
name: "OPSX: Ship Code"
description: Execute a ship-plan handoff task-by-task — Red→Green→one commit per change task, then verify/evidence/sync/changelog/PR
category: Workflow
tags: [workflow, automation, tdd, pr, experimental]
---

Execute the `.handoff/<change>/` handoff produced by `/opsx:ship-plan`, **task by
task and test-first**. For each change task it writes the failing test (Red),
implements the minimal code to pass (Green), and makes **one commit containing
both**. After all tasks it runs the full verify gates, writes evidence, syncs the
delta specs, updates the changelog, and opens (or updates) a PR. This is the second
half of `/opsx:ship`. **Stops at PR opened** — a human merges.

**Input**: Optionally a change name (e.g., `/opsx:ship-code c0006-…`). `--dry-run`
makes the per-task commits locally but skips push + PR. `--only <pair>` runs a single
pair (e.g. `--only 02`); `--retry-blocked` re-runs blocked pairs.

**Steps**

1. **Select the change** and confirm a handoff exists: check
   `.handoff/<name>/plan.json`. If missing, tell the user to run
   `/opsx:ship-plan <name>` first and stop.

2. **Approval gate.** Summarize the handoff (pairs + their test/code deliverables)
   and a clean-tree note. Use **AskUserQuestion**: "Run ship-code on this handoff?"
   with options *Ship it*, *Dry run (commits, no push/PR)*, *Cancel*. Don't proceed
   without an explicit choice (unless the user already said to).

3. **Launch the Workflow** (date from context):
   ```
   Workflow({ name: 'ship-code', args: { change: '<name>', date: '<YYYY-MM-DD>', dryRun: <bool>, only: '<pair?>', retryBlocked: <bool?> } })
   ```
   Phases: **Preflight** (toolchain check, validate, clean tree, branch
   `feat/<name>`, load handoff) → **Implement** (per pair: Red → Green → one commit)
   → **Verify** (`go build`/`make vet`/`make test` + coverage + `openspec validate`,
   repair loop) → **Evidence** (`openspec/changes/<name>/evidence/`) → **Sync** →
   **Changelog** (+ chore commit) → **PR**.

4. **Relay the result.** Report the branch, the **per-task commits** (one per change
   task, each red+green), the chore commit, verify gates + coverage, evidence dir,
   and the **PR URL**. On dry run: the local commits to inspect (`git log --stat`).
   On a blocked pair / failed verify / budget stop: surface the `stage` + `reason`;
   completed commits are on the branch.

**Guardrails**
- Never launch without the gate in step 2; never without a handoff.
- Each change task = exactly one commit (the failing test + the implementation).
- Does not merge or archive. After merge, run `/opsx:archive <name>`. For reviewer
  feedback, use `/opsx:address-review`.
- The Preflight toolchain check stops early if `go` is too old (< 1.25) or
  `openspec`/`gh` are missing.
