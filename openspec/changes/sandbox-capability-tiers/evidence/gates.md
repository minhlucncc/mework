# Gates — sandbox-capability-tiers

## Compile + static analysis gates

| toolchain | unitDir | gate | command | result |
|-----------|---------|------|---------|--------|
| go | libs/client | build | `go build ./libs/client/...` | PASS |
| go | libs/mcp-server | build | `go build ./libs/mcp-server/...` | PASS |
| go | libs/sandbox | build | `go build ./libs/sandbox/...` | PASS |
| go | libs/shared | build | `go build ./libs/shared/...` | PASS |
| go | libs/client | vet | `go vet ./libs/client/...` | PASS |
| go | libs/mcp-server | vet | `go vet ./libs/mcp-server/...` | PASS |
| go | libs/sandbox | vet | `go vet ./libs/sandbox/...` | PASS |
| go | libs/shared | vet | `go vet ./libs/shared/...` | PASS |

## Test gates (race enabled)

| toolchain | unitDir | gate | command | result |
|-----------|---------|------|---------|--------|
| go | libs/client | test -race | `go test -race ./libs/client/...` | PASS |
| go | libs/mcp-server | test -race | `go test -race ./libs/mcp-server/...` | PASS |
| go | libs/sandbox | test -race | `go test -race ./libs/sandbox/...` | PASS |
| go | libs/shared | test -race | `go test -race ./libs/shared/...` | PASS |

## Spec validation gates

| toolchain | unitDir | gate | command | result |
|-----------|---------|------|---------|--------|
| openspec | change | validate --strict | `npx openspec validate sandbox-capability-tiers --strict` | PASS |
| openspec | change | free bench ladder | `node .claude/workflows/lib/openspec.js validate sandbox-capability-tiers --strict` | FAIL (4 pre-existing spec formatting errors, 1 warning — not related to this change) |

## Commits

| unit | sha | description |
|------|-----|-------------|
| 01 | `1525b13` | Add AccessTier type to core types and SandboxBundleMetadata |
| 02 | `8c8b752` | Honor AccessTier in local engine, propagate through sandbox creation, orchestrator starts as observer, spawned workers get worker tier |
| 03 | `11b293e` | Update template metadata with access tier and orchestrator CLAUDE.md observer guidance |
| repair | `3117279` | fix: resolve verify gate failures for sandbox-capability-tiers |

- **Repair count**: 1 (commit 3117279)
- **Governing skills**: (none)

## llmGates

No llmGates tier configuration present in this repository. Not applicable.
