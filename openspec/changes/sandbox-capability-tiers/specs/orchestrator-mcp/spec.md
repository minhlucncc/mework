# MODIFIED

## Purpose

Update the orchestrator MCP specification so that child sandboxes spawned via
the `spawn_sandbox` tool are created with the worker tier, granting full
read-write access within the workspace.

## Requirements

### MODIFIED: Requirement: Sandbox lifecycle MCP tools

**Before:**
The MCP server SHALL expose five sandbox lifecycle tools: `spawn_sandbox`,
`get_sandbox_status`, `list_child_sandboxes`, `destroy_sandbox`, and
`wait_for_sandbox`. These SHALL call the existing `ports.SandboxDriver` /
`sandbox/runtime.Manager` interfaces and track child sandboxes in an in-memory
registry scoped to the MCP server process.

**After:**
The MCP server SHALL expose five sandbox lifecycle tools: `spawn_sandbox`,
`get_sandbox_status`, `list_child_sandboxes`, `destroy_sandbox`, and
`wait_for_sandbox`. These SHALL call the existing `ports.SandboxDriver` /
`sandbox/runtime.Manager` interfaces and track child sandboxes in an in-memory
registry scoped to the MCP server process. When `RealSandboxManager.Start()` is
called to create a child sandbox, it SHALL set `RunSpec.AccessTier` to `worker`
so that spawned workers get full read-write access within the workspace. The
caller MUST NOT be able to override the tier for spawned children through the
MCP tool parameters.

#### Scenario: Spawn a child sandbox with worker tier

- **WHEN** `RealSandboxManager.Start()` is called to create a child sandbox
- **THEN** `RunSpec.AccessTier` is set to `AccessWorker`
- **AND** the spawned sandbox has `SandboxCaps().AccessTier` equal to `worker`

#### Scenario: Spawned sandbox has full filesystem access

- **WHEN** a child sandbox is created with AccessTier `worker`
- **THEN** the sandbox has read-write access within its workspace directory

#### Scenario: Spawn async preserves stdin-not-argv invariant

- **WHEN** `spawn_sandbox` is called with a `prompt` string
- **THEN** a new sandbox starts with AccessTier `worker` via the runtime manager
- **AND** the prompt is fed to the child over stdin (not argv)
