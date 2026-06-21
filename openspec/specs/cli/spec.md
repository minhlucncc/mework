# CLI Specification

## Purpose

Define the `mework` command-line surface and configuration model: managing Mello
boards/tickets/comments, registering runtimes and profiles on the server,
connecting providers, and controlling the daemon. Owned by `cmd/mework` and
`internal/cli`.

## Requirements

### Requirement: Command surface

The system SHALL provide commands grouped as: Core (provider task management:
`workspace list`; `board list/get`; `ticket list/get/create/move`;
`comment list/add`; `search`), Runner (`runner enroll` for install-once
enrollment; `daemon start/stop/status/restart/logs`; read-only `agent list`
and `session list` to inspect dispatched agents and active sessions), and
Additional (`login`; `auth status/logout`; `config show/set`; `provider connect`;
`version`). Read commands SHALL support `--json` output. The poll-oriented
`runtime register` / claim framing is replaced by `runner enroll`.

#### Scenario: Enroll a runner from the CLI

- **WHEN** an operator runs `mework runner enroll --url <hub> --token <registration-token>`
- **THEN** the runner is enrolled with a durable identity and is ready to receive dispatches unattended

#### Scenario: Inspect active sessions

- **WHEN** an operator runs `mework session list --json`
- **THEN** the CLI emits the active sessions for the enrolled runner as JSON

#### Scenario: Machine-readable output

- **WHEN** a user passes `--json` to a list/get command
- **THEN** the command emits JSON suitable for scripting

### Requirement: Configuration resolution

The system SHALL resolve configuration with precedence **flag > environment >
config file**, storing the config file at `~/.mework/config.json`. The
`--profile <name>` flag (or `MEWORK_PROFILE`) MUST isolate config, daemon state,
pid, and logs under `~/.mework/profiles/<name>/`, and `MEWORK_HOME` MUST override
the root directory.

#### Scenario: Flag overrides env and file

- **WHEN** a value is set in the config file, the environment, and a flag
- **THEN** the flag value is used

#### Scenario: Profile isolation

- **WHEN** a command runs with `--profile work`
- **THEN** it reads and writes state under `~/.mework/profiles/work/` instead of the default location

### Requirement: Credential file safety

The system SHALL persist tokens and config with restrictive permissions
(`0600` for files, `0700` for directories) so local credentials are not
world-readable.

#### Scenario: Token stored with restrictive permissions

- **WHEN** the CLI writes a token to the config
- **THEN** the file is created with `0600` permissions
