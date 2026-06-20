---
name: "OPSX: Ship Plan"
description: Plan an approved OpenSpec change as a reviewable .handoff/<change>/ — 2 tasks (test + code) per change task
category: Workflow
tags: [workflow, automation, planning, tdd, experimental]
---

Turn an **approved** OpenSpec change into a reviewable **execution handoff** under
`.handoff/<change>/` — without writing any code. For every task in the change's
`tasks.md` it emits **two** handoff tasks: a **test/Red** task (the test plan) and a
**code/Green** task. `/opsx:ship-code` later executes them, one red+green commit per
change task. This is the first half of `/opsx:ship`.

**Input**: Optionally a change name (e.g., `/opsx:ship-plan c0006-…`). If omitted,
infer from context / `openspec list --json` + AskUserQuestion.

**Steps**

1. **Select the change** (announce "Planning change: <name>").

2. **Confirm it's plannable.** Run `openspec status --change "<name>" --json` and
   `openspec validate "<name>"`. Show the open task count from `tasks.md`. If the
   change isn't approved/validated, say so and stop.

3. **Launch the Workflow** (today's date from context, `YYYY-MM-DD`):
   ```
   Workflow({ name: 'ship-plan', args: { change: '<name>', date: '<YYYY-MM-DD>' } })
   ```
   It writes `.handoff/<name>/plan.json`, `README.md`, and
   `tasks/NN-a-test.md` + `NN-b-code.md` (2 per change task). `.handoff/` is
   gitignored — it's local execution scaffolding.

4. **Show the handoff for review.** Report the handoff path and the per-pair
   breakdown (each change task → its test deliverable + code deliverable). Invite
   the user to inspect/edit the task files before running `/opsx:ship-code`.

**Guardrails**
- Planning only — no branch, no code, no commits.
- Idempotent: re-running preserves any handoff tasks already marked `done`.
- If `tasks.md` has no open tasks, report nothing to plan.
