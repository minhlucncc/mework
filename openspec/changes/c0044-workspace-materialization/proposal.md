## Why

Workspace **base materialization** (git clone / archive unpack / store copy) and **hook
execution** are stubs in `libs/server/storage/manager.go` (the `[stub] hook … would run`
path, ~lines 198-241). So a workspace-bound session that declares a base (a repo/archive to
start from) or lifecycle hooks doesn't actually materialize code or run those hooks (H7) —
the sandbox only sees an already-present local dir. To make workspace sessions production-real
(clone the repo, run setup hooks), materialization must be implemented.

## What Changes

- **Materialize a workspace base.** Implement the three base kinds:
  - **git** — clone (optionally a ref/branch, shallow) into the workspace dir;
  - **archive** — download + unpack (tar/zip) into the workspace dir;
  - **store** — copy from the configured object store (the `c0043` artifact/object store).
  Materialization is idempotent and bounded (size/time limits), and runs **on the runner**
  (the c0027 boundary — the server never materializes source).
- **Execute lifecycle hooks.** Run declared setup hooks (e.g. `postCreate`) in the workspace
  dir with the agent's environment, **stdin-not-argv** for any provided input, capturing
  output and failing the session setup on a non-zero hook exit.
- **Surface failures.** Materialization/hook failures produce a clear session-setup error
  (reported via the existing result path) rather than silently starting an empty sandbox.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `prebuilt-agent-sandbox`: a workspace-bound session SHALL **materialize its declared base**
  (git/archive/store) into the workspace directory and **run its declared setup hooks** before
  the agent's first turn, on the runner; failures abort session setup with a clear error.

## Impact

- **Server/runner storage:** `libs/server/storage/manager.go` (replace the materialization +
  hook stubs with real implementations), reusing the object store for the `store` base kind
  (depends on `c0043`).
- **Schema/config:** the workspace config (`mework.yml`) gains optional `base` (kind + source +
  ref) and `hooks` fields, decoded into the existing metadata.
- **Tests:** git (local bare repo fixture), archive (in-memory tar/zip), and store-copy
  materialization; a hook that writes a file and one that fails (non-zero → setup error);
  size/time bound enforced.
- **Depends on** `c0043` (object store) for the `store` base kind. Preserves stdin-not-argv,
  one-agent-per-sandbox, and the runner-side-execution boundary.
