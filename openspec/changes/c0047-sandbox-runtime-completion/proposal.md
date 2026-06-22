## Why

Three sandbox-runtime pieces are stubs (M3/M4):

- **`mework-sandbox` binary** (`libs/sandbox/cmd/mework-sandbox/main.go`) runs in "stub mode"
  — it logs and blocks, with no engine selection or execution surface. It's the intended
  out-of-process sandbox runner for container/remote engines but does nothing.
- **Cloudflare engine** (`libs/sandbox/engine/cloudflare`) implements only remote `Exec`;
  `Mount`/`Signals`/`Stop`/`Destroy` are no-ops/errors, so it can't run a real bound-workspace
  session.
- **`permission` package** (`libs/server/permission`) is an empty stub (grant enforcement
  currently lives only in middleware), leaving a confusing dead package.

`local` and `docker` engines are real and carry the implemented paths; this change completes
the remaining engine surface and the binary so the sandbox layer is coherent.

## What Changes

- **Real `mework-sandbox` binary.** Implement engine selection from config/flags and a defined
  execution surface (start a sandbox for a given definition + workspace, exec turns over
  stdin, stream output, stop/destroy) so it is a usable out-of-process runner, not a blocking
  stub. Graceful shutdown + clear errors.
- **Complete the Cloudflare engine lifecycle.** Implement `Mount` (push workspace to the
  remote sandbox), `Signals`/`Stop`/`Destroy` (remote lifecycle), so a Cloudflare-engine
  session has the same lifecycle contract as `docker` — or, where a remote capability is
  genuinely unavailable, return a typed "unsupported" error the manager handles gracefully
  (no silent no-ops).
- **Resolve the `permission` package.** Either implement the intended server-side grant
  enforcement helpers there (and have the middleware call them) or remove the empty package
  and document that enforcement lives in `middleware` — eliminating the dead stub. (Decision in
  design.)

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `prebuilt-agent-sandbox`: the sandbox runtime surface is **complete** — the
  `mework-sandbox` runner binary selects an engine and drives the sandbox lifecycle, and every
  registered engine either fully implements the lifecycle (start/mount/exec/signal/stop/
  destroy) or returns a typed unsupported error (no silent no-ops).

## Impact

- **Sandbox:** `libs/sandbox/cmd/mework-sandbox/main.go` (real binary),
  `libs/sandbox/engine/cloudflare/*` (lifecycle), engine capability reporting.
- **Server:** `libs/server/permission` (implement-or-remove; if removed, update references).
- **Tests:** sandbox binary smoke (engine selection, start→exec→stop with the `local` engine);
  cloudflare lifecycle against a stub remote; capability/“unsupported” behavior asserted.
- **Build:** add `mework-sandbox` to the Makefile build targets (it was absent).
- Preserves stdin-not-argv and one-agent-per-sandbox. No schema migration.
