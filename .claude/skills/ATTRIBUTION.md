# Skill attribution

The engineering-practice skills in this directory are **adapted from**
[addyosmani/agent-skills](https://github.com/addyosmani/agent-skills) (MIT License),
a library of production-grade engineering skills for AI coding agents.

Each vendored skill keeps the upstream methodology and structure, localized to this
repository's stack and conventions (Go 1.25.x, OpenSpec `/opsx:*` lifecycle, `make`
targets, the job-queue/provider-gateway invariants, evidence under
`openspec/changes/<name>/evidence/`). See [docs/engineering-skills.md](../../docs/engineering-skills.md)
for the lifecycle map and how the skills compose.

## Vendored from upstream (adapted)

- `using-agent-skills` — meta-router / operating rules
- `spec-driven-development` — mapped onto OpenSpec
- `planning-and-task-breakdown`
- `test-driven-development`
- `incremental-implementation`
- `debugging-and-error-recovery`
- `code-review-and-quality`
- `code-simplification`
- `security-and-hardening`
- `git-workflow-and-versioning`
- `ci-cd-and-automation`
- `documentation-and-adrs`

Upstream license: MIT, © Addy Osmani and contributors. This adaptation retains the
MIT terms; see the upstream `LICENSE` for the full text.
