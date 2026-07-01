# MODIFIED

## Purpose

Update the offline agent specification to require that the offline-mode
agent (orchestrator) runs in an observer-tier sandbox with restricted
capabilities.

## Requirements

### MODIFIED: Requirement: User can start an offline-mode agent bound to a workspace directory

**Before:**
The system SHALL provide a `mework start --workspace <dir> --offline` command
that boots a self-contained local agent process. The agent SHALL resolve its
definition from `<dir>/mework.yml` using a file-system resolver, start the
sandbox (local engine), and listen for one-shot task submissions via the CLI.
The command SHALL fail with a clear error when `--workspace` is missing.

**After:**
The system SHALL provide a `mework start --workspace <dir> --offline` command
that boots a self-contained local agent process. The agent SHALL resolve its
definition from `<dir>/mework.yml` using a file-system resolver, start the
sandbox (local engine) with AccessTier `observer`, and listen for one-shot task
submissions via the CLI. The command SHALL fail with a clear error when
`--workspace` is missing.

#### Scenario: Start offline agent with valid workspace — observer tier

- **WHEN** user runs `mework start --workspace /tmp/my-workspace --offline`
- **THEN** the agent reads `/tmp/my-workspace/mework.yml` for the agent
  definition
- **AND** the agent starts the local sandbox with AccessTier `observer`
- **AND** `SandboxCaps().AccessTier` returns `observer`
- **AND** the agent prints its status and waits for task submissions

#### Scenario: Observer-tier offline agent scopes working directory

- **WHEN** the offline agent starts with AccessTier `observer`
- **THEN** the sandbox working directory is bound to the workspace directory
- **AND** the sandbox CLAUDE.md includes observer-mode guidance
