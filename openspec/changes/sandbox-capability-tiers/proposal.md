---
name: "sandbox-capability-tiers"
schema: spec-driven
---

# Sandbox capability tiers — observer, worker, isolated

## What

Replace the binary offline/online runner distinction with a unified **sandbox
capability tier** model. Every agent runs in a sandbox with a well-defined
access tier that determines what it can do.

## Why

- **Runner offline and runner online are the same thing** — both are just
  sandboxes. No distinction needed.
- **The orchestrator currently runs unprotected** — no sandbox isolation at
  all. A destructive command (`rm -rf /`) in the orchestrator can destroy the
  host.
- **Future sandboxing tech** (Docker, Landlock, seccomp) needs a slot to plug
  into. The tier model defines the contract now so stronger isolation drops in
  transparently later.

## Tiers

| Tier | Filesystem | Commands | Spawn child sandboxes | Target engine |
|------|-----------|----------|----------------------|-------------|
| **observer** | read-only workspace | read-only (ls, grep, cat, find) + MCP | spawn_sandbox ✅ | local (restricted) |
| **worker** | read-write workspace | all (build, test, edit, commit) | ❌ (depth capped) | local / docker |
| **isolated** | read-write workspace | all | ❌ | docker (container) |

## Scope

**In scope:**
- `AccessTier` type in `SandboxCaps` and `RunSpec`
- `AccessTier` field in `SandboxBundleMetadata`
- Local engine honors observer tier (cwd scope, self-enforcement via prompt)
- Orchestrator starts in observer tier
- Spawned workers get worker tier

**Out of scope (future):**
- Docker engine integration for isolated tier
- Landlock / seccomp / user namespace isolation
- Runtime enforcement in the local engine beyond cwd + prompt
- Renaming / deprecating the offline daemon concept

## Assumptions

- Local engine cannot enforce real OS-level isolation without Docker/root.
  The observer tier on local relies on agent self-restriction via CLAUDE.md +
  MCP tools as the write path. Real isolation comes from the docker engine.
- The SandboxCaps reports the tier so consumers know the contract — they
  don't need to guess based on engine name.
