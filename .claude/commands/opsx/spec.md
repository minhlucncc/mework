---
name: "OPSX: Spec"
description: Author and quality-assure an OpenSpec change spec — cross-validate across the 6 review axes, then revise until clean
category: Workflow
tags: [workflow, automation, spec, review, experimental]
---

Take an OpenSpec change's spec from draft to **clean, minimal, testable, and
complete** before any code is written. This is the authoring counterpart of
`/opsx:ship`: it loads a change (or scaffolds and drafts one), fans out one
read-only **critic per spec-review axis in parallel**, then runs a **revise loop**
that fixes Blocker/Required findings and re-runs `openspec validate --strict`
until the spec is clean. It writes `openspec/changes/<name>/review/REVIEW.md` and
stops — a human then runs `/opsx:apply` or `/opsx:ship`.

Use this after `/opsx:propose` (the fast single-pass draft) and before
implementation. The quality bar is the `spec-review-and-quality` skill (six axes:
Structure/validity · Clarity/KISS · Testability · Minimality/YAGNI ·
Consistency/DRY · Completeness).

**Input**: Optionally specify a change name (e.g. `/opsx:spec c0002-message-bus`).
If omitted, infer from context, auto-select if only one active change exists,
otherwise run `openspec list --json` and use **AskUserQuestion** to pick. To draft
a brand-new change, pass a slug (e.g. `/opsx:spec --new github-provider-adapter`);
the workflow picks the next `cNNNN-` number. Pass `--dry-run` to review only (no
edits — just the findings + report).

**Steps**

1. **Select the target.** Either an existing change name, or — for a new spec — a
   kebab slug to draft. Announce: "Reviewing spec: <name>" (or "Drafting + reviewing
   new change: <slug>") and how to override.

2. **Gate (AskUserQuestion).** Confirm intent before launching the autonomous run:
   - For an existing change, run `openspec status --change "<name>" --json` and show
     a one-line summary (title, delta specs, task count).
   - Ask: "Review and revise this spec?" with options like *Review & revise*,
     *Review only (dry run — no edits)*, *Cancel*. Do not proceed without a choice.
     (If the user already said to review/revise, skip the prompt but show the
     summary.)

3. **Preflight sanity (cheap).** If revising (not dry-run), confirm `git status
   --porcelain` is clean enough that the spec edits will be reviewable; the Workflow
   re-checks. For a brand-new draft, confirm the slug isn't already taken.

4. **Launch the Workflow.** Today's date is required (the script cannot call the
   system clock); use `currentDate` from context (`YYYY-MM-DD`):

   ```
   Workflow({ name: 'spec-change', args: { change: '<name>', slug: '<slug|undefined>', date: '<YYYY-MM-DD>', dryRun: <true|false>, maxRevisions: <2> } })
   ```

   - For an **existing** change, pass `change: '<name>'` and omit `slug`.
   - To **draft a new** change, pass the intended `change: 'cNNNN-<slug>'` (or let
     preflight assign the number) **and** `slug: '<slug>'` so the workflow scaffolds
     it; this only happens when `dryRun` is false.

   The Workflow runs: **Preflight** (status + validate; load or scaffold) →
   **Cross-validate** (one read-only critic per axis, in parallel; axis 1 runs
   `openspec validate --strict`) → **Revise** (fix Blocker/Required, re-validate,
   loop ≤ `maxRevisions`; skipped on dry-run) → **Report**
   (`review/REVIEW.md` + verdict).

5. **Relay the result.** Report the Workflow's `stage`/`ok` and:
   - On **approve**: the verdict, revisions run, that `openspec validate` passes, the
     **review report path**, and the next step — `/opsx:ship <name>` or `/opsx:apply`.
   - On **revise** (Blocker/Required remain after `maxRevisions`, or dry-run): the
     open-finding count and the report path; tell the user to read the findings and
     fix them (or re-run with a higher `maxRevisions`).
   - On early stop (preflight failure / budget reserve): surface `stage` and
     `reason` verbatim.

**Guardrails**
- Never launch without the gate in step 2.
- `dryRun: true` makes **no edits** — use it to inspect findings before committing
  to an auto-revise run.
- This command edits only the change's artifacts under `openspec/changes/<name>/`;
  it does not implement code, sync specs, or open a PR. After approval, hand off to
  `/opsx:apply` or `/opsx:ship`.
- Scaffolding a new change requires a `slug` and a non-dry run; otherwise a missing
  change aborts in preflight.
