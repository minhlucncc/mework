## ADDED Requirements

### Requirement: Workspace base materialization and setup hooks

A workspace-bound session SHALL **materialize its declared base** into the workspace directory
before the agent's first turn, supporting at least **git** (clone, optional ref), **archive**
(download and unpack, path-traversal-safe), and **store** (copy from the configured object
store). Materialization SHALL run on the runner (never the server), be idempotent (skip when
already materialized), and be bounded in size and time. A session MAY declare **setup hooks**
that run in the workspace directory before the first turn with input delivered over stdin (not
argv); a hook that exits non-zero SHALL abort session setup with a clear error. A workspace
that declares neither a base nor hooks SHALL behave as before (bind the existing directory).

#### Scenario: Git base is cloned into the workspace

- **WHEN** a session opens with a git base
- **THEN** the runner clones it into the workspace directory before the first turn

#### Scenario: Archive base is unpacked safely

- **WHEN** a session opens with an archive base
- **THEN** the runner unpacks it into the workspace directory without allowing path traversal

#### Scenario: Setup hook failure aborts setup

- **WHEN** a declared setup hook exits non-zero
- **THEN** session setup fails with the hook's error surfaced, and no agent turn runs

#### Scenario: No base or hooks preserves current behavior

- **WHEN** a workspace declares neither a base nor hooks
- **THEN** the session binds the existing workspace directory exactly as before
