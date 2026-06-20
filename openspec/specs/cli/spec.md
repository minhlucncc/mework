# CLI Specification

## Purpose

Define the `mework` command-line surface and configuration model: managing Mello
boards/tickets/comments, registering runtimes and profiles on the server,
connecting providers, and controlling the daemon. Owned by `cmd/mework` and
`internal/cli`.

## Requirements

### Requirement: Command surface

The system SHALL provide commands grouped as: Core
(`workspace list`; `board list/get`; `ticket list/get/create/move`;
`comment list/add`; `search`), Runtime
(`daemon start/stop/status/restart/logs`; `runtime register/list/revoke`;
`profile create/list/update/delete`), and Additional
(`login`; `auth status/logout`; `config show/set`; `provider connect`;
`version`). Read commands SHALL support `--json` output.

#### Scenario: Register a runtime

- **WHEN** a user runs `mework runtime register --code local-macbook`
- **THEN** the server creates a runtime and returns a one-time runtime token (`rt_token`)

#### Scenario: Create a profile

- **WHEN** a user runs `mework profile create --name default --backend claude ...`
- **THEN** the server stores an AI instruction profile for the account

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
