---
name: git-workflow-and-versioning
description: Structures git workflow practices, adapted for the mework Go project. Use when making any code change. Use when committing, branching, resolving conflicts, writing Conventional Commit messages, opening PRs via gh, maintaining CHANGELOG.md, or organizing work across parallel streams.
---

# Git Workflow and Versioning

## Overview

Git is your safety net. Treat commits as save points, branches as sandboxes, and history as documentation. With AI agents generating code at high speed, disciplined version control is the mechanism that keeps changes manageable, reviewable, and reversible.

## When to Use

Always. Every code change flows through git.

## Core Principles

### Trunk-Based Development (Recommended)

Keep `main` always deployable. Work in short-lived feature branches that merge back within 1-3 days. Long-lived development branches are hidden costs — they diverge, create merge conflicts, and delay integration. DORA research consistently shows trunk-based development correlates with high-performing engineering teams.

```
main ──●──●──●──●──●──●──●──●──●──  (always deployable)
        ╲      ╱  ╲    ╱
         ●──●─╱    ●──╱    ← short-lived feature branches (1-3 days)
```

In `mework`, this maps directly onto the OpenSpec change lifecycle: one OpenSpec
change → one `feat/<change-name>` branch → one PR. **Never commit on `main`** —
branch first. The `/opsx:ship` workflow automates the commit → push → PR step at
the end of a change (see "mework notes").

- **Dev branches are costs.** Every day a branch lives, it accumulates merge risk.
- **Release branches are acceptable.** When you need to stabilize a release while main moves forward.
- **Feature flags > long branches.** Prefer deploying incomplete work behind flags rather than keeping it on a branch for weeks.

### 1. Commit Early, Commit Often

Each successful increment gets its own commit. Don't accumulate large uncommitted changes.

```
Work pattern:
  Implement slice → make test → make vet → Commit → Next slice

Not this:
  Implement everything → Hope it works → Giant commit
```

Commits are save points. If the next change breaks something, you can revert to the last known-good state instantly. When applying an OpenSpec change with `/opsx:apply`, tick each task in `tasks.md` and commit per logical slice.

### 2. Atomic Commits

Each commit does one logical thing:

```
# Good: Each commit is self-contained
git log --oneline
a1b2c3d feat(jobs): add heartbeat extension to claim transaction
d4e5f6g feat(webhook): parse workflow keyword in ParseTrigger
h7i8j9k test(jobs): cover claimed→running transition
m1n2o3p docs: update job-queue spec for heartbeat change

# Bad: Everything mixed together
x1y2z3a add jobs feature, fix webhook, update deps, refactor secret
```

### 3. Descriptive Messages — Conventional Commits

This repo uses **Conventional Commits**. Messages explain the *why*, not just the *what*, and **every commit ends with the Co-Authored-By trailer**:

```
# Good: Explains intent, Conventional Commit type, trailer
feat(webhook): match workflow keyword in trigger grammar

Adds plan|cook|test|review|ship|journal recognition to ParseTrigger so
the server can route a job to the right AI workflow. Keeps the legacy
/run keyword untouched to avoid breaking the local-daemon path.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>

# Bad: Describes what's obvious from the diff, no trailer
update parse.go
```

**Format:**
```
<type>(<scope>): <short description>

<optional body explaining why, not what>

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
```

**Types:**
- `feat` — New feature
- `fix` — Bug fix
- `refactor` — Code change that neither fixes a bug nor adds a feature
- `test` — Adding or updating tests
- `docs` — Documentation only
- `chore` — Tooling, dependencies, config

Common scopes mirror the repo layout: `jobs`, `webhook`, `daemon`, `agentrun`,
`provider`, `secret`, `cli`, `store`.

### 4. Keep Concerns Separate

Don't combine formatting changes with behavior changes. Don't combine refactors with features. Each type of change should be a separate commit — and ideally a separate PR / OpenSpec change:

```
# Good: Separate concerns
git commit -m "refactor(secret): extract seal/unseal helper"
git commit -m "feat(connection): seal provider credentials on write"

# Bad: Mixed concerns
git commit -m "refactor secret and add credential sealing"
```

**Separate refactoring from feature work.** A refactoring change and a feature change are two different changes — submit them separately. This makes each change easier to review, revert, and understand in history. Small cleanups (renaming a variable) can be included in a feature commit at reviewer discretion.

### 5. Size Your Changes

Target ~100 lines per commit/PR. Changes over ~1000 lines should be split.

```
~100 lines  → Easy to review, easy to revert
~300 lines  → Acceptable for a single logical change
~1000 lines → Split into smaller changes
```

A well-scoped OpenSpec change usually lands in this range. If a proposal's
`tasks.md` implies a >1000-line diff, consider splitting it into multiple
changes during `/opsx:propose`.

## Branching Strategy

### Feature Branches

```
main (always deployable)
  │
  ├── feat/message-bus       ← One OpenSpec change per branch
  ├── feat/agent-catalog     ← Parallel work
  └── fix/heartbeat-race     ← Bug fixes
```

- Branch from `main` (the default branch)
- Keep branches short-lived (merge within 1-3 days) — long-lived branches are hidden costs
- Delete branches after merge
- Prefer feature flags over long-lived branches for incomplete features

### Branch Naming

```
feat/<change-name>         → feat/message-bus            (matches the OpenSpec change name)
fix/<short-description>     → fix/heartbeat-race
chore/<short-description>   → chore/bump-pgx
refactor/<short-description>→ refactor/secret-module
docs/<short-description>    → docs/openspec-workflow
```

Remote is `git@github.com:dnd288/mework.git`.

## Working with Worktrees

For parallel AI agent work, use git worktrees to run multiple branches simultaneously:

```bash
# Create a worktree for a feature branch
git worktree add ../mework-message-bus feat/message-bus
git worktree add ../mework-agent-catalog feat/agent-catalog

# Each worktree is a separate directory with its own branch
ls ../
  mework/                  ← main branch
  mework-message-bus/      ← message-bus change
  mework-agent-catalog/    ← agent-catalog change

# When done, merge and clean up
git worktree remove ../mework-message-bus
```

> Caveat for DB-backed work: `make test` runs with `-p 1` against a single
> shared Postgres (`TEST_DATABASE_URL`). Two worktrees running `make test` at
> once will collide on the same database — point each at a separate test DB or
> serialize the runs.

Benefits:
- Multiple agents can work on different changes simultaneously
- No branch switching needed (each directory has its own branch)
- If one experiment fails, delete the worktree — nothing is lost
- Changes are isolated until explicitly merged

## The Save Point Pattern

```
Agent starts work
    │
    ├── Makes a change
    │   ├── make test passes? → Commit → Continue
    │   └── make test fails?  → Revert to last commit → Investigate
    │
    └── Change complete → All commits form a clean history
```

This pattern means you never lose more than one increment of work. If an agent goes off the rails, `git reset --hard HEAD` takes you back to the last successful state.

## Change Summaries

After any modification, provide a structured summary. This makes review easier, documents scope discipline, and surfaces unintended changes:

```
CHANGES MADE:
- internal/server/jobs/state.go: Added running→queued requeue transition
- internal/server/jobs/state_test.go: Table case for requeue

THINGS I DIDN'T TOUCH (intentionally):
- internal/server/jobs/sweeper.go: Could reuse requeue but out of scope
- internal/daemon/poll.go: Backoff tuning is a separate change

POTENTIAL CONCERNS:
- Requeue resets heartbeat — confirm the sweeper won't immediately re-sweep.
- No migration needed (status is already a text column).
```

The "DIDN'T TOUCH" section shows you exercised scope discipline and didn't go on an unsolicited renovation.

## Pre-Commit Hygiene

Before every commit:

```bash
# 1. Check what you're about to commit
git diff --staged

# 2. Ensure no secrets (this repo seals provider creds; never commit raw keys)
git diff --staged | grep -iE "password|secret|api_key|rt_token|MEWORK_SECRET_KEY|SERVER_KEY"

# 3. Run tests (serialized; needs TEST_DATABASE_URL for DB-backed tests)
make test

# 4. Vet
make vet

# 5. Build both binaries
make build      # or: go build ./...
```

There is no JS/husky toolchain here. If you want a local guard, a simple
`.git/hooks/pre-commit` that runs `make vet` and `gofmt -l` is sufficient.

## Handling Generated Files

- **Commit** `go.mod` / `go.sum`, embedded goose migrations under
  `internal/store/migrations/*.sql`, and OpenSpec artifacts under `openspec/`.
- **Don't commit** build output (`bin/`, goreleaser `dist/`), local config
  (`~/.mework/config.json` lives outside the repo), or any `.env` with real
  secrets.
- **`.gitignore`** should cover at least: `bin/`, `dist/`, `*.env`, and editor
  cruft.

## Using Git for Debugging

```bash
# Find which commit introduced a bug
git bisect start
git bisect bad HEAD
git bisect good <known-good-commit>
# At each midpoint, run: make test

# View what changed recently
git log --oneline -20
git diff HEAD~5..HEAD -- internal/server/jobs/

# Find who last changed a specific line
git blame internal/server/jobs/state.go

# Search commit messages for a keyword
git log --grep="heartbeat" --oneline
```

## CHANGELOG Discipline

The repo keeps a root `CHANGELOG.md` in **Keep a Changelog** format — one bullet
per shipped change. **`/opsx:ship` generates the entry for you** when shipping an
approved change; you rarely hand-edit it. The shape:

```markdown
# Changelog

## [Unreleased]
### Added
- Webhook trigger grammar now routes the `journal` workflow.

### Fixed
- Heartbeat race that could double-claim a runtime.
```

## Opening a PR

`mework` uses the `gh` CLI. `/opsx:ship` runs this for you; to do it by hand:

```bash
gh pr create --base main --title "feat(jobs): heartbeat extension" --body "$(cat <<'EOF'
## Summary
- ...

## Test plan
- make vet
- make test

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

`/opsx:ship` **stops at PR opened — it does not auto-merge.** Archiving the change
with `/opsx:archive` is a separate, post-merge human step.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll commit when the feature is done" | One giant commit is impossible to review, debug, or revert. Commit each slice. |
| "The message doesn't matter" | Messages are documentation. Use Conventional Commits + the Co-Authored-By trailer. |
| "I'll squash it all later" | Squashing destroys the development narrative. Prefer clean incremental commits. |
| "Branches add overhead" | Short-lived branches are free. Long-lived branches are the problem — merge within 1-3 days. |
| "I'll just commit on main" | Don't. Branch first (`feat/<change>`); `main` stays deployable. |
| "I'll split this change later" | Large changes are harder to review, riskier to deploy, and harder to revert. Split before submitting. |

## Red Flags

- Large uncommitted changes accumulating
- Commit messages like "fix", "update", "misc" — or missing the Co-Authored-By trailer
- Commits made directly on `main`
- Formatting changes mixed with behavior changes
- Committing `bin/`, `dist/`, or a `.env` with real `SERVER_KEY` / `MEWORK_SECRET_KEY`
- Long-lived branches that diverge significantly from main
- Force-pushing to shared branches

## mework notes

- One OpenSpec change → one `feat/<change>` branch → one PR. Drive non-trivial
  work through `/opsx:propose` → `/opsx:apply` → `/opsx:sync` → `/opsx:archive`.
- **`/opsx:ship`** is the autonomous lane: apply → verify (`make vet` / `make
  test` + `openspec validate`) → sync delta specs → prepend the `CHANGELOG.md`
  entry → commit (with the Co-Authored-By trailer) → push → open the PR via `gh`,
  then **STOPS at the PR** (no auto-merge). `--dry-run` stops before push/PR.
- `/opsx:archive` runs **after** the PR merges, moving the change to
  `openspec/changes/archive/`.
- `/opsx:address-review` processes PR review comments via `gh` — use it to turn
  reviewer feedback into follow-up commits on the same branch.
- Capture verification evidence under `openspec/changes/<name>/evidence/`.
- Remote: `git@github.com:dnd288/mework.git`. See the sibling
  `ci-cd-and-automation` skill for the CI gates these commits must pass, and
  `documentation-and-adrs` for when a change needs an ADR / spec update.

## Verification

For every commit:

- [ ] Commit does one logical thing
- [ ] Message uses a Conventional Commit type and explains the why
- [ ] Message ends with the `Co-Authored-By: Claude Opus 4.8 (1M context)` trailer
- [ ] On a `feat/`/`fix/` branch, not on `main`
- [ ] `make test` and `make vet` pass before committing
- [ ] No secrets in the diff
- [ ] No formatting-only changes mixed with behavior changes
- [ ] `.gitignore` covers `bin/`, `dist/`, `.env`
