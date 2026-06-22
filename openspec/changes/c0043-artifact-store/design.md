## Context

`hub/artifacts.go` has a `DummyArtifactStore` returning errors; `libs/server/storage` already
provides a driver-pluggable `Manager` (`fs`/`s3`/`minio`/`r2`) wired from `cfg.Storage` in
`LoadConfig`. The routes exist but serve the dummy.

## Goals / Non-Goals

**Goals:** real persistence + retrieval of run artifacts via the configured object store;
per-run namespacing; safe names; auth-scoped download.

**Non-Goals:** artifact lifecycle/GC/retention policy; large-object streaming optimizations;
content scanning. (Note for retention/GC as future work.)

## Decisions

- **Adapter over `storage.Manager`, not a new backend.** The artifact store is a thin adapter
  translating (runID, name) → object key `runs/<runID>/artifacts/<sanitized-name>` and
  delegating to the configured driver. No new storage code.
- **Default `fs`.** Keeps dev/local and the existing tests working without S3/minio; prod sets
  `STORAGE_*`.
- **Presign or proxy by driver capability.** If the driver supports presigned URLs (s3/r2),
  expose them; otherwise stream the object through the server. Fall back gracefully.
- **Path-traversal safe.** Reject/sanitize `name` (no `..`, no separators) before building the
  key; downloads check the run's tenant/owner via the auth context.

## Risks / Trade-offs

- **[fs driver has no presign]** → proxy-stream for fs; presign only where supported. The API
  shape is the same to clients.
- **[No retention/GC]** → artifacts accumulate; flagged as future work (lifecycle policy).
- **[Large artifacts]** → stream rather than buffer; rely on the storage driver's reader.

## Migration Plan

Additive. Swaps the dummy for a real store keyed off existing `cfg.Storage`. No DB migration;
existing deployments default to `fs`.
