# Tasks — sandbox-capability-tiers

## Task [1]: Add AccessTier type to core types  (tags: backend, db, migration)

Add `AccessTier` type with constants `AccessObserver`, `AccessWorker`,
`AccessIsolated` to `libs/shared/core/types.go`. Add `AccessTier` field to
`SandboxCaps` and `RunSpec`.

The default rule for empty AccessTier is defined in the design (see design.md
Data model section): empty string MUST be treated as `AccessWorker`.

Verification: `go vet ./libs/shared/core/...` passes.

## Task [2]: Add AccessTier to SandboxBundleMetadata  (tags: backend)

Add `AccessTier core.AccessTier` field to `SandboxBundleMetadata` in
`libs/sandbox/schema.go`. Empty string defaults per the design.md rule
(empty string MUST be treated as `AccessWorker` at resolution time when
the bundle is loaded and validated).

Update `Validate()` to reject unknown tier values.

Verification: `go test ./libs/sandbox/...` passes.

## Task [3]: Local engine honors AccessTier  (tags: backend)

Update `libs/sandbox/engine/local/runner.go`:

- `Start()`: read `spec.AccessTier`. For observer tier, tag the sandbox as
  restricted.
- `Exec()`: for observer tier, run commands with workspace-scoped working
  directory. The injection-safety invariant (stdin, not argv) is unchanged.
- `Caps()`: include the access tier so consumers can inspect it.

Verification: `go test ./libs/sandbox/...` and `./libs/client/runner/...` pass.

## Task [4]: Propagate AccessTier through sandbox creation  (tags: backend)

Update `libs/sandbox/runtime/manager.go` — pass `spec.AccessTier` through to
the driver.

Verification: tests pass.

## Task [5]: Orchestrator starts as observer  (tags: backend, cli)

Update `libs/client/cli/daemon.go` — in `runOfflineForeground`, set
`spec.AccessTier = core.AccessObserver` when starting the orchestrator
sandbox. The local engine will then scope its execution accordingly.

Update `libs/client/runner/interactive_session.go` — propagate the access
tier from `RunSpec` through `OpenSession`.

Verification: `mework agent send mybot "list files"` works; orchestrator
observes.
- [ ] Task [5]

## Task [6]: Spawned workers get worker tier  (tags: backend, mcp)

Update `libs/mcp-server/sandbox.go` — in `RealSandboxManager.Start()`, set
`spec.AccessTier = core.AccessWorker` so spawned worker sandboxes get full
read-write access.

Verification: `mework agent send mybot "spawn sandbox to write a file"`
works.
- [ ] Task [6]

## Task [7]: Template metadata  (tags: docs, config)

Update `templates/workspace/orchestrator/mework.yml` to include
`access: observer`.

Update `templates/workspace/worker/mework.yml` to include `access: worker`.

Update `examples/remote-claude/.mework/orchestrator/mework.yml` for
consistency.

Verification: `openspec validate --strict` passes.

## Task [8]: Orchestrator CLAUDE.md observer guidance  (tags: docs)

Update `templates/workspace/orchestrator/CLAUDE.md` to include the observer
mode section from the design: read-only commands + MCP tools for writes,
no direct destructive operations.

Update `examples/remote-claude/.mework/orchestrator/CLAUDE.md` for
consistency.

Verification: grep the CLAUDE.md for "observer mode" guidance text.

## Task checklist

- [x] Task [1]
- [x] Task [2]
- [ ] Task [3]
- [ ] Task [4]
- [ ] Task [5]
- [ ] Task [6]
- [ ] Task [7]
- [ ] Task [8]
