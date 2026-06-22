## Why

The artifact store is a stub: `NewDummyArtifactStore()` (`libs/server/hub/artifacts.go`) makes
every Put/Get/PresignURL return `"artifact store not yet wired"`, yet the
`/api/v1/runs/{runID}/artifacts*` routes are live (`hub/router.go`). So run artifacts cannot
be stored or retrieved (H6). The object-storage backends already exist and are pluggable
(`libs/server/storage` with `fs`, `s3`, `minio`, `r2` drivers + a `Manager`), so this is a
wiring + thin-adapter job, not new infrastructure.

## What Changes

- **Back the artifact store with the configured object store.** Replace the dummy with an
  `ObjectStore`-backed implementation that uses `storage.Manager` (driver from `STORAGE_*`
  config, default `fs`). Put/Get/List/Presign map to the storage backend under a stable
  artifact key layout (e.g. `runs/<runID>/artifacts/<name>`).
- **Wire it in the router.** `hub.NewServer` constructs the real store from `cfg.Storage`
  instead of the dummy; the `/runs/{runID}/artifacts` and `/runs/{runID}/artifacts/{name}`
  routes serve real list/download.
- **Tenant/run scoping + safety.** Artifact keys are namespaced by run; download enforces the
  run's tenant/owner via the existing auth context; names are sanitized to prevent path
  traversal in the key.

## Capabilities

### New Capabilities
- `artifact-store`: run artifacts are persisted to and served from the configured object
  storage backend (fs/s3/minio/r2), namespaced per run, replacing the non-functional dummy.

### Modified Capabilities
<!-- none -->

## Impact

- **Server:** `libs/server/hub/artifacts.go` (real `ObjectStore`-backed store + handlers),
  `libs/server/hub/router.go` (construct from `cfg.Storage`). Reuses `libs/server/storage`
  (`Manager`, drivers).
- **Tests:** artifact put→list→get round-trip against the `fs` driver (temp dir); traversal-
  sanitization test; unauthorized cross-tenant download denied.
- No schema migration (object storage, not DB). Default `fs` driver keeps local/dev working
  with no extra services.
