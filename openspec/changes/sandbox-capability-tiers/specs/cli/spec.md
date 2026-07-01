## MODIFIED Requirements

### Requirement: Command surface

The system SHALL provide commands grouped as: Core (provider task management:
`workspace list`; `board list/get`; `ticket list/get/create/move`;
`comment list/add`; `search`), Runner (`runner enroll` for install-once
enrollment; `daemon start/stop/status/restart/logs`; read-only `agent list`;
`server start` to run the hub in-process; a `session` group to inspect and drive
interactive sessions — `session list`, `session create`, `session send`,
`session attach`, `session close`; and a `sandbox` group — `sandbox start`,
`sandbox list`, `sandbox stop`, `sandbox send`), and Additional (`login`; `auth
status/logout`; `config show/set`; `provider connect`; `version`). Read commands
SHALL support `--json` output. When invoked with `--offline`, `mework daemon
start` SHALL create the orchestrator sandbox with AccessTier `observer`. The
`sandbox start` command SHALL report the AccessTier of the created child sandbox.

#### Scenario: daemon start --offline creates observer-tier sandbox

- **WHEN** the user runs `mework daemon start --offline --workspace <dir>`
- **THEN** the orchestrator sandbox starts with AccessTier `observer`
- **AND** `SandboxCaps.AccessTier` returns `observer`

#### Scenario: sandbox start reports AccessTier

- **WHEN** the user runs `mework sandbox start --workspace .`
- **THEN** the CLI reports both the sandbox id and its AccessTier value

## ADDED Requirements

### Requirement: Observer tier enforces cwd scoping

The local engine SHALL bind the sandbox working directory to the configured
workspace path when the orchestrator starts with AccessTier `observer` via
`mework daemon start --offline`. The local engine SHALL NOT enforce OS-level
command filtering for the observer tier; the agent SHALL be instructed to
self-restrict via CLAUDE.md observer-mode guidance. The sandbox SHALL report
the AccessTier through `SandboxCaps()`.

#### Scenario: Observer sandbox working directory is workspace-bound

- **WHEN** the orchestrator sandbox starts with AccessTier `observer`
- **THEN** the sandbox working directory is set to the workspace directory
- **AND** `SandboxCaps().AccessTier` returns `observer`

#### Scenario: Observer sandbox does not filter OS commands

- **WHEN** `Exec("rm -rf /")` is called on an observer-tier sandbox in the
  local engine
- **THEN** the command executes (the local engine does not implement OS-level
  filtering)
- **AND** the sandbox CLAUDE.md provides observer-mode guidance instructing
  read-only behavior
