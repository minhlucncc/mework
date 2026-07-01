# Design — sandbox-capability-tiers

## Architecture

```
                    ACCESS TIER
                        │
      ┌─────────────────┼─────────────────┐
      │                 │                 │
  observer           worker           isolated
      │                 │                 │
      ▼                 ▼                 ▼
┌──────────┐     ┌──────────┐     ┌──────────┐
│  local   │     │  local   │     │  docker  │
│ (cwd scope)│     │ (cwd)    │     │ (full)   │
└──────────┘     └──────────┘     └──────────┘
      │                 │                 │
      ▼                 ▼                 ▼
 Read tools +    All tools +        All tools +
 MCP spawn       no MCP spawn       no MCP spawn
```

## Data model

### `libs/shared/core/types.go` — Add AccessTier

```go
// AccessTier defines what an agent in a sandbox is permitted to do.
type AccessTier string

const (
    AccessObserver AccessTier = "observer"  // read-only workspace + read commands + MCP
    AccessWorker   AccessTier = "worker"    // full read-write within workspace
    AccessIsolated AccessTier = "isolated"  // container-isolated
)

// SandboxCaps gains an AccessTier field.
type SandboxCaps struct {
    // ... existing fields ...
    AccessTier AccessTier
}

// RunSpec gains an AccessTier field so callers request a specific tier.
type RunSpec struct {
    // ... existing fields ...
    AccessTier AccessTier
}
```

### `libs/sandbox/schema.go` — Add AccessTier to bundle metadata

```go
type SandboxBundleMetadata struct {
    // ... existing fields ...
    AccessTier core.AccessTier  // "observer", "worker", or "isolated"
}
```

When `AccessTier` is the empty string (whether from Go zero value, YAML
omission, or deserialization), it MUST be treated as `AccessWorker`. This
normalization SHALL be applied in a single location such as a constructor or
accessor method on the type (backward compatible — existing sandboxes continue
to have full access).

## Engine behavior by tier

### Local engine (libs/sandbox/engine/local/runner.go)

```
observer:
  - cwd = workspace directory
  - Exec runs in workspace (path validation for write ops)
  - CLAUDE.md instructs read-only behavior
  - MCP tools provide the write path

worker:
  - cwd = workspace directory (existing behavior)
  - Full access within workspace
  - No changes from current behavior
```

### Docker engine (future)

```
isolated:
  - Container-level isolation
  - Resource limits
  - No host filesystem access
```

## Propagation

### Orchestrator startup (libs/client/cli/daemon.go)

```go
// runOfflineForeground starts the orchestrator as an observer sandbox.
spec := core.RunSpec{
    AgentID:     meta.Name,
    BackendName: meta.Backend,
    Workspace:   core.Workspace{Path: workspaceDir},
    AccessTier:  core.AccessObserver,   // ← NEW
}
```

### Worker spawning (libs/mcp-server/sandbox.go)

```go
// Spawned workers get the worker tier.
func (m *RealSandboxManager) Start(ctx context.Context, spec core.RunSpec) (ports.Sandbox, error) {
    spec.AccessTier = core.AccessWorker  // ← NEW
    return m.client.CreateSession(ctx, spec)
}
```

### Templates

The `templates/workspace/orchestrator/mework.yml` gains:
```yaml
name: orchestrator
access: observer  # ← NEW
```

The `templates/workspace/worker/mework.yml` gains:
```yaml
name: worker
access: worker  # ← NEW
```

## Agent self-enforcement

The observer tier's write restriction is enforced through the agent's CLAUDE.md:

```markdown
## Observer mode

You are running in **observer** mode. You can:
- Read files and search the codebase
- Run read-only commands (grep, cat, ls, find)
- Use MCP tools (spawn_sandbox, write_artifact) for write operations

You must NOT:
- Modify or delete files directly
- Run destructive commands (rm -rf, chmod, sudo)
- Write outside the workspace directory

If you need to make changes, spawn a worker sandbox via `spawn_sandbox`.
```
