## MODIFIED Requirements

### Requirement: Safe, isolated execution

The system SHALL feed the prompt (and, for interactive sessions, **each turn**) to
the backend over **stdin, never as a shell argument** (ticket/turn content is
attacker-controllable), execute in a working directory whose scope depends on the
sandbox `AccessTier`, and bound each run by a timeout. A **one-shot** run is
bounded by a per-run timeout (default 30 minutes). A **long-lived interactive**
sandbox is instead bounded by its **session lifecycle** — explicit close or idle
reaping — rather than a single per-run timeout, while individual turns MAY still
carry a per-turn bound. The sandbox `RunSpec` and `SandboxCaps` SHALL carry an
`AccessTier` field that determines the capability level: `observer` (read-only,
cwd-scoped), `worker` (full read-write within workspace), or `isolated`
(container-isolated, future). When `AccessTier` is the empty string, it SHALL be
treated as `worker`.

#### Scenario: RunSpec with AccessTier creates a scoped sandbox

- **WHEN** `RunSpec` with `AccessTier: observer` is passed to
  `LocalEngine.Start()`
- **THEN** the started sandbox has `SandboxCaps().AccessTier` equal to
  `observer`
- **AND** the sandbox working directory is bound to the workspace path

#### Scenario: Empty AccessTier defaults to worker

- **WHEN** `RunSpec` is created with no `AccessTier` set (zero value)
- **THEN** the sandbox starts with `SandboxCaps().AccessTier` equal to
  `worker`

#### Scenario: Prompt stays off argv for observer-tier sandboxes

- **WHEN** the daemon runs an observer-tier sandbox with ticket-derived
  prompt content
- **THEN** the prompt is written to the process stdin and never appears in
  argv (the stdin-not-argv invariant is unchanged regardless of tier)

## ADDED Requirements

### Requirement: Sandbox capability tier type

The system SHALL define an `AccessTier` type in `libs/shared/core/types.go`
with constants `AccessObserver`, `AccessWorker`, and `AccessIsolated`. The
`RunSpec` and `SandboxCaps` structs SHALL each gain an `AccessTier` field.
The `SandboxBundleMetadata` struct in `libs/sandbox/schema.go` SHALL also
gain an `AccessTier` field. `Validate()` on `SandboxBundleMetadata` SHALL
reject unknown `AccessTier` values (any string other than `"observer"`,
`"worker"`, `"isolated"`, or the empty string).

#### Scenario: AccessTier constants compile and match expected values

- **WHEN** Go code references the AccessTier type
- **THEN** the constants `core.AccessObserver`, `core.AccessWorker`, and
  `core.AccessIsolated` are available with values `"observer"`, `"worker"`,
  and `"isolated"` respectively

#### Scenario: Validate rejects unknown AccessTier

- **WHEN** a `SandboxBundleMetadata` with `AccessTier: "superuser"` is
  validated
- **THEN** `Validate()` returns an error indicating the invalid AccessTier
  value

#### Scenario: Validate accepts valid AccessTier values

- **WHEN** a `SandboxBundleMetadata` with `AccessTier: "observer"` is
  validated
- **THEN** `Validate()` returns no error for the AccessTier field
