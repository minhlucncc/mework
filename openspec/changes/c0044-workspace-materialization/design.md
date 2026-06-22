## Context

`storage/manager.go` has stubbed base materialization and hook execution. Workspace-bound
sessions (c0036) bind a dir but never populate it from a declared base or run setup hooks.

## Goals / Non-Goals

**Goals:** real git/archive/store materialization into the workspace dir; real hook execution
with proper failure semantics; bounded + idempotent; runner-side.

**Non-Goals:** incremental sync / live file watching (pack/push/pull remains the whole-
workspace transfer); arbitrary hook sandboxing beyond the engine's existing isolation;
private-repo credential management beyond what the connection/grant already provides (note as
follow-up if needed).

## Decisions

- **Three base kinds behind one interface.** `git` (clone, optional shallow + ref), `archive`
  (download + unpack tar/zip with a path-traversal-safe extractor — reuse the workspace
  extractor pattern), `store` (copy objects from the `c0043` object store by prefix). Selected
  by the workspace config's `base.kind`.
- **Idempotent + bounded.** If the workspace dir is already materialized (marker/non-empty),
  skip re-clone; enforce max size + a clone/unpack timeout to avoid runaway materialization.
- **Hooks: stdin-not-argv, fail-closed.** Run each hook in the workspace dir; pass any input on
  stdin; a non-zero exit aborts session setup with the captured stderr surfaced as the setup
  error. Hooks run before the first turn.
- **Runner-side only.** Materialization + hooks execute where the sandbox runs (the runner),
  never on the server, preserving the c0027 boundary.

## Risks / Trade-offs

- **[Arbitrary clone/unpack = DoS surface]** → size + time bounds; traversal-safe unpack;
  document trusted-source assumption for the `local` engine.
- **[Private repo auth]** → use credentials from the connection/grant where available; out of
  scope to add a new secret path here (flagged).
- **[Hook arbitrary code]** → runs within the engine's isolation (container engines isolate;
  `local` is trusted-only, unchanged).

## Migration Plan

Additive: `base`/`hooks` are optional config; workspaces without them behave exactly as today
(bind the existing dir, no hooks). No DB migration.
