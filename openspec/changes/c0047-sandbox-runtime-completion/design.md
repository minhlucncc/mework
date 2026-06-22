## Context

`local`/`docker` engines and the runtime manager are real; the `mework-sandbox` binary is a
blocking stub, the `cloudflare` engine is Exec-only, and `libs/server/permission` is empty.
These leave the sandbox layer half-finished and confusing.

## Goals / Non-Goals

**Goals:** a usable `mework-sandbox` runner; a coherent engine lifecycle contract (no silent
no-ops); resolve the dead `permission` package.

**Non-Goals:** the `custom` engine's external plugin (it is intentionally an indirection that
errors without a registered plugin — leave as-is, documented); a new remote provider beyond
Cloudflare; reworking grant semantics (only relocate/clarify enforcement).

## Decisions

- **`mework-sandbox` drives the runtime manager.** The binary parses an engine + definition +
  workspace, builds a `core.RunSpec`, and uses `runtime.NewManagerFor(engine)` to
  start/exec/stop — the same path the in-process runner uses — exposing it as a standalone
  process for container/remote operation. stdin-not-argv preserved.
- **Engines: implement or typed-unsupported, never silent no-op.** Cloudflare gains real
  `Mount`/`Stop`/`Destroy`/`Signals` where the remote API allows; where a capability is truly
  absent, return a typed `ErrUnsupported` the `runtime.Manager` surfaces clearly. Engine
  `Caps()` advertises what's supported so callers can pre-check.
- **`permission` package: remove, don't fake.** Grant enforcement already lives in
  `middleware` (`GrantMiddleware`/`RequireOperation`) and `shared/grant`. The empty
  `libs/server/permission` adds confusion with no behavior — **remove it** and ensure nothing
  imports it (it's unused). (If a future server-side helper is wanted, it can be reintroduced
  with real code.)

## Risks / Trade-offs

- **[Cloudflare remote lifecycle limits]** → typed unsupported + `Caps()` keeps behavior
  honest rather than pretending; the manager handles it.
- **[Removing `permission` if something imports it]** → verify zero importers first
  (assessment found none); the removal is safe and reduces confusion.
- **[Sandbox binary scope creep]** → keep it a thin driver over `runtime.Manager`; no new
  orchestration logic.

## Migration Plan

Additive for the binary/engine; the `permission` removal is a safe deletion of an unused empty
package. Makefile gains a `mework-sandbox` build target. No schema migration.
