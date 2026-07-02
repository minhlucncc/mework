---
name: worker-mzspec
description: How to run the mzspec SDLC pipeline inside a mework sandbox. Use when implementing, spec'ing, or shipping a change.
tags: [mzspec, mework]
---

# Worker mzspec Skill

You are a **worker agent** spawned in a mework sandbox to execute part of the
mzspec SDLC pipeline. Your workspace is the project directory. You communicate
results via stdout (which the orchestrator reads).

## Available commands

```bash
/opsx:propose "<description>"   # Scaffold a new change
/opsx:spec <change>              # 6-axis spec review
/opsx:spec-pr <change>           # Open SPEC PR
/opsx:ship-plan <change>         # Plan TDD work-units
/opsx:ship-code <change>         # Implement test-first, open CODE PR
/opsx:sync <change>              # Sync delta specs
/opsx:archive <change>           # Archive completed change
/opsx:author-review              # Multi-dimension code review
/opsx:address-review <change>    # Address PR feedback
```

## Sandbox constraints

- **stdin-not-argv**: content is fed over stdin, never on the command line
- **Workspace**: mounted at the workspace path from `spawn_sandbox()`
- **MCP tools**: `get_session_context()`, `write_artifact()`, `notify_human()`

## Reporting results

When your task is complete, output a JSON summary line for the orchestrator:

```json
{"status":"done|failed","change":"cNNNN-slug","spec_pr":"url","code_pr":"url","summary":"..."}
```
