## 1. Base materialization (TDD)

- [ ] 1.1 Tests: `git` (clone a local bare-repo fixture into the workspace), `archive` (unpack
      an in-memory tar/zip, traversal-safe), `store` (copy from the object store by prefix).
- [ ] 1.2 Implement the three base kinds in `storage/manager.go`, replacing the stub; idempotent
      (skip if already materialized) and bounded (max size + timeout).

## 2. Hook execution (TDD)

- [ ] 2.1 Tests: a setup hook that writes a file into the workspace runs; a hook that exits
      non-zero aborts setup with the stderr surfaced; input is delivered on stdin (not argv).
- [ ] 2.2 Implement hook execution in the workspace dir before the first turn; fail-closed.

## 3. Config + wiring

- [ ] 3.1 Extend the workspace config (`mework.yml`) with optional `base` (kind/source/ref) and
      `hooks`; decode into the existing metadata; absent → today's behavior.
- [ ] 3.2 Invoke materialization + hooks on the runner during session/sandbox open.

## 4. Validation

- [ ] 4.1 `make vet` + `make test` green.
- [ ] 4.2 `openspec validate c0044-workspace-materialization --strict` passes.
