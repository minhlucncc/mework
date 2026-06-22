## ADDED Requirements

### Requirement: Object-store-backed run artifacts

The system SHALL persist and serve run artifacts using the configured object-storage backend
(filesystem by default; S3/MinIO/R2 when configured), namespaced per run. Storing an artifact
SHALL place it under a per-run key, and listing/downloading SHALL return the stored content.
Artifact names SHALL be sanitized so a name cannot escape its run's key prefix
(path-traversal safe), and downloads SHALL be authorized against the run's tenant/owner.

#### Scenario: Artifact round-trip

- **WHEN** an artifact is stored for a run and then listed and downloaded
- **THEN** the listing includes it and the download returns the stored content

#### Scenario: Default filesystem backend works without extra services

- **WHEN** no object-storage driver is configured
- **THEN** the system uses the filesystem backend and artifact store/retrieve still works

#### Scenario: Traversal-unsafe name is rejected

- **WHEN** an artifact name contains path-traversal sequences or separators
- **THEN** the system rejects or sanitizes it so the stored key stays within the run prefix

#### Scenario: Cross-tenant download denied

- **WHEN** a caller requests an artifact for a run outside its tenant
- **THEN** the system denies the download
